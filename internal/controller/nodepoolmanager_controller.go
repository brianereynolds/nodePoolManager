/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	"github.com/brianereynolds/k8smanagers_utils"
	k8smanagersv1 "greyridge.com/nodePoolManager/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NodePoolManagerReconciler reconciles a NodePoolManager object
type NodePoolManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	DefaultAzureCredential *azidentity.DefaultAzureCredential
	ManagedClustersClient  *armcontainerservice.ManagedClustersClient
	AgentPoolClient        *armcontainerservice.AgentPoolsClient
}

// getValidAgentPoolVersions gets a list of valid versions that this node pool can be upgraded to
func (r *NodePoolManagerReconciler) getValidAgentPoolVersions(ctx context.Context, npManager *k8smanagersv1.NodePoolManager, apClient *armcontainerservice.AgentPoolsClient) string {
	var builder strings.Builder
	// Invalid version - get list of good ones
	resp, err := apClient.GetAvailableAgentPoolVersions(ctx, npManager.Spec.ResourceGroup, npManager.Spec.ClusterName, nil)
	if err == nil {
		for _, version := range resp.Properties.AgentPoolVersions {
			builder.WriteString(*version.KubernetesVersion)
			builder.WriteString(" ")
		}
	}
	return builder.String()
}

// nodePoolExists will check if a node pool exists which matches the provided name.
// It returns true if the node pool already exists (Status object), otherwise it returns false.
func (r *NodePoolManagerReconciler) nodePoolExists(ctx context.Context, npManager *k8smanagersv1.NodePoolManager, checkPoolName string) bool {
	l := log.Log

	for i := 0; i < len(npManager.Status.NodePools); i++ {
		pool := npManager.Status.NodePools[i]

		l.V(1).Info("nodePoolExists: Checking", "pool.Name", pool.Name, "checkPoolName", checkPoolName)
		if pool.Name == checkPoolName {
			return true
		}
	}
	return false
}

// nodePoolIsNewer will check the Node Pool Version in the provided object versus the current state (Status object)
// It will return true if the version in Spec is newer than the Status, otherwise it returns false
func (r *NodePoolManagerReconciler) nodePoolIsNewer(ctx context.Context, npManager *k8smanagersv1.NodePoolManager, targetPool k8smanagersv1.NodePool) bool {
	l := log.Log

	for i := 0; i < len(npManager.Status.NodePools); i++ {
		currentPool := npManager.Status.NodePools[i]

		if currentPool.Name == targetPool.Name {
			l.Info("nodePoolIsNewer: Comparing", "currentPool.Version", currentPool.Props.OrchestratorVersion, "targetPool.Version", targetPool.Props.OrchestratorVersion)
			result, err := k8smanagers_utils.CompareVersions(*targetPool.Props.OrchestratorVersion, *currentPool.Props.OrchestratorVersion)

			if err != nil {
				l.Error(err, "Failure calling compareVersions")
			}

			if (result == 1) || (result == 0) {
				return true
			}
			if result == -1 {
				l.Info("nodePoolIsNewer: value in YAML is older than existing node pool", "name", currentPool.Name,
					"YAML", targetPool.Props.OrchestratorVersion,
					"System", currentPool.Props.OrchestratorVersion)
			}
		}
	}
	return false
}

// refreshNodePools will use the Agent Pools Client to connect to the AKS cluster and add the current list of node pools to the Status object
func (r *NodePoolManagerReconciler) refreshNodePools(ctx context.Context, client *armcontainerservice.AgentPoolsClient, rgroup string, clusterName string, status *k8smanagersv1.NodePoolManagerStatus) error {
	l := log.Log
	l.Info("Enter refreshNodePools", "rgroup", rgroup, "clusterName", clusterName)

	// TODO: this should probably clear out the status.NodePools before as it's using append

	// List agent pools
	pager := client.NewListPager(rgroup, clusterName, nil)
	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			panic(err.Error())
		}
		for _, agentPool := range page.AgentPoolListResult.Value {

			l.Info("refreshNodePools: Cluster", "Name", *agentPool.Name, "Type", *agentPool.Type, "Version", *agentPool.Properties.CurrentOrchestratorVersion)

			l.V(1).Info("refreshNodePools: Cluster Details", "ID", agentPool.ID,
				"EnableAutoScaling", agentPool.Properties.EnableAutoScaling,
				"AvailabilityZones", agentPool.Properties.AvailabilityZones,
				"MinCount", agentPool.Properties.MinCount,
				"MaxCount", agentPool.Properties.MaxCount,
				"MaxPods", agentPool.Properties.MaxPods,
				"Mode", agentPool.Properties.Mode,
				"PowerState", agentPool.Properties.PowerState)

			status.NodePools = append(status.NodePools, k8smanagersv1.NodePool{
				Name:  *agentPool.Name,
				Props: *agentPool.Properties})
		}
	}

	return nil
}

// populateStatus is a wrapper method to add the latest state to the Status object
func (r *NodePoolManagerReconciler) populateStatus(ctx context.Context, npManager *k8smanagersv1.NodePoolManager) error {
	spec := npManager.Spec

	managedClusterClient, err := k8smanagers_utils.GetManagedClusterClient(ctx, spec.SubscriptionID)
	if err != nil {
		return err
	}

	cluster, err := managedClusterClient.Get(context.Background(), spec.ResourceGroup, spec.ClusterName, nil)
	if err != nil {
		return err
	}

	npManager.Status.ClusterVersion = *cluster.Properties.KubernetesVersion

	apClient, err := k8smanagers_utils.GetAgentPoolClient(ctx, spec.SubscriptionID)
	if err != nil {
		return err
	}

	err = r.refreshNodePools(ctx, apClient, spec.ResourceGroup, spec.ClusterName, &npManager.Status)
	if err != nil {
		return err
	}

	return nil
}

// populateSpec will enrich the Spec object with relevant information.
func (r *NodePoolManagerReconciler) populateSpec(ctx context.Context, npManager *k8smanagersv1.NodePoolManager) error {
	l := log.Log
	if len(npManager.Status.NodePools) == 0 {
		err := errors.New("there are no node pools in Status. Has the object been initialized correctly")
		l.Error(err, "populateSpec failed")
		return err
	}

	for i := 0; i < len(npManager.Spec.NodePools); i++ {
		// Populate the Action, if not done already
		if npManager.Spec.NodePools[i].Action != "" {
			// Must be already set via the YAML
			continue
		}

		if r.nodePoolExists(ctx, npManager, npManager.Spec.NodePools[i].Name) == false {
			npManager.Spec.NodePools[i].Action = k8smanagersv1.Create
		} else if r.nodePoolIsNewer(ctx, npManager, npManager.Spec.NodePools[i]) {
			npManager.Spec.NodePools[i].Action = k8smanagersv1.Update
		}
	}

	return nil
}

// createNodePools is a method that will validate and create new node pools if they do not already exist
func (r *NodePoolManagerReconciler) createNodePools(ctx context.Context, npManager *k8smanagersv1.NodePoolManager) error {
	for i := 0; i < len(npManager.Spec.NodePools); i++ {
		pool := npManager.Spec.NodePools[i]

		if pool.Action == k8smanagersv1.Create {
			err := r.doCreateNodePool(ctx, npManager, pool)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// updateNodePools is a method that will validate and run an upgrade on any node pools that exist and have a newer version specified in the YAML
func (r *NodePoolManagerReconciler) updateNodePools(ctx context.Context, npManager *k8smanagersv1.NodePoolManager) error {
	for i := 0; i < len(npManager.Spec.NodePools); i++ {
		pool := npManager.Spec.NodePools[i]

		if pool.Action == k8smanagersv1.Update {
			err := r.doUpdateNodePool(ctx, npManager, pool)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// deleteNodePools is a method which will validate and delete the node pool if the appropriate action is provided in the YAML
func (r *NodePoolManagerReconciler) deleteNodePools(ctx context.Context, npManager *k8smanagersv1.NodePoolManager) error {
	for i := 0; i < len(npManager.Spec.NodePools); i++ {
		pool := npManager.Spec.NodePools[i]

		if pool.Action == k8smanagersv1.Delete {
			err := r.doDeleteNodePool(ctx, npManager, pool.Name)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// doCreateNodePool is a worker method that will use Agent Pool Client to create the provided node pool
func (r *NodePoolManagerReconciler) doCreateNodePool(ctx context.Context, npManager *k8smanagersv1.NodePoolManager, pool k8smanagersv1.NodePool) error {
	l := log.Log
	l.Info("doCreateNodePool", "nodePoolName", pool.Name)
	spec := npManager.Spec

	if spec.TestMode {
		l.Info("TEST MODE: The controller will try to create this node pool", "name", pool.Name)
		return nil
	}

	apClient, err := k8smanagers_utils.GetAgentPoolClient(ctx, spec.SubscriptionID)
	if err != nil {
		return err
	}

	agentPool := armcontainerservice.AgentPool{
		Name:       to.Ptr(pool.Name),
		Properties: to.Ptr(pool.Props),
	}
	agentPool.Properties.Mode = to.Ptr(armcontainerservice.AgentPoolModeUser)

	poller, err := apClient.BeginCreateOrUpdate(ctx, npManager.Spec.ResourceGroup, npManager.Spec.ClusterName,
		pool.Name, agentPool, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			if respErr.StatusCode == 400 {
				l.Error(respErr, "Error message")
				/*l.Error(err, "Invalid version specified", "version", pool.Props.OrchestratorVersion,
				"Valid versions", r.getValidAgentPoolVersions(ctx, npManager, apClient))*/
			}
		}
		l.Error(err, "Cannot initialize upgrade")
		return err
	}

	l.Info("Starting node pool creation")

	// Wait for the operation to complete
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		l.Error(err, "doCreateNodePool: Failure when polling upgrade status")
		return err
	}

	l.Info("doCreateNodePool: Node pool created successfully", "name", pool.Name, "version", pool.Props.OrchestratorVersion)
	return nil

}

// doUpdateNodePool is a worker method that will update the provided node pool
func (r *NodePoolManagerReconciler) doUpdateNodePool(ctx context.Context, npManager *k8smanagersv1.NodePoolManager, pool k8smanagersv1.NodePool) error {
	l := log.Log
	l.Info("doUpdateNodePool", "nodePoolName", pool.Name)
	spec := npManager.Spec

	if spec.TestMode {
		l.Info("TEST MODE: The controller will try to update this node pool", "name", pool.Name, "targetVersion", pool.Props.OrchestratorVersion)
		return nil
	}

	apClient, err := k8smanagers_utils.GetAgentPoolClient(ctx, spec.SubscriptionID)
	if err != nil {
		return err
	}

	agentPool, err := apClient.Get(ctx, npManager.Spec.ResourceGroup, npManager.Spec.ClusterName, pool.Name, nil)
	if err != nil {
		l.Error(err, "doUpdateNodePool: Failed to get AgentPool object")
		return err
	}

	if agentPool.Properties.OrchestratorVersion == pool.Props.OrchestratorVersion {
		l.Info("node pool versions are the same. No upgrade will be performed")
	}

	agentPool.Properties.OrchestratorVersion = pool.Props.OrchestratorVersion
	if pool.Props.MinCount != nil {
		agentPool.Properties.MinCount = pool.Props.MinCount
	}
	if pool.Props.MaxCount != nil {
		agentPool.Properties.MaxCount = pool.Props.MaxCount
	}
	if pool.Props.VMSize != nil {
		agentPool.Properties.VMSize = pool.Props.VMSize
	}

	// Upgrade the node pool
	poller, err := apClient.BeginCreateOrUpdate(ctx, npManager.Spec.ResourceGroup, npManager.Spec.ClusterName,
		pool.Name, agentPool.AgentPool, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			if respErr.StatusCode == 400 {
				l.Error(err, "Invalid version specified", "version", pool.Props.OrchestratorVersion,
					"Valid versions", r.getValidAgentPoolVersions(ctx, npManager, apClient))
			}
		}
		l.Error(err, "Cannot initialize upgrade")
		return err
	}

	l.Info("Starting node pool update")

	// Wait for the operation to complete
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		l.Error(err, "doUpdateNodePool: Failure when polling upgrade status")
		return err
	}

	l.Info("doUpdateNodePool: Update complete successfully", "name", pool.Name, "version", pool.Props.OrchestratorVersion)
	return nil
}

// doDeleteNodePool is a worker method that will delete the node pool with the provided name.
func (r *NodePoolManagerReconciler) doDeleteNodePool(ctx context.Context, npManager *k8smanagersv1.NodePoolManager, nodePoolName string) error {
	l := log.Log
	l.Info("doDeleteNodePool", "nodePoolName", nodePoolName)
	spec := npManager.Spec

	if spec.TestMode {
		l.Info("TEST MODE: The controller will try to delete this node pool", "name", nodePoolName)
		return nil
	}

	apClient, err := k8smanagers_utils.GetAgentPoolClient(ctx, spec.SubscriptionID)
	if err != nil {
		return err
	}

	poller, err := apClient.BeginDelete(ctx, spec.ResourceGroup, spec.ClusterName, nodePoolName, nil)
	if err != nil {
		var notFoundErr *azcore.ResponseError
		if errors.As(err, &notFoundErr) && notFoundErr.StatusCode == 404 {
			l.Error(err, "Node pool not found", "name", nodePoolName)
			return err
		} else {
			l.Error(err, "Failure in starting node pool delete")
			return err
		}
	}

	_, err = poller.PollUntilDone(context.Background(), nil)
	if err != nil {
		l.Error(err, "Failure in polling on node pool delete", "err", err)
		return err
	}

	l.Info("doDeleteNodePool: Node Pool deleted successfully!", "name", nodePoolName)
	return nil
}

// validate method will verify all parameters are ship shape and bristol
func (r *NodePoolManagerReconciler) validate(ctx context.Context, npManager *k8smanagersv1.NodePoolManager) error {
	l := log.Log

	//spew.Dump(npManager.Spec)
	//spew.Dump(npManager.Status)

	for i := 0; i < len(npManager.Spec.NodePools); i++ {
		np := npManager.Spec.NodePools[i]

		if np.Action == k8smanagersv1.Create {
			if np.Props.VMSize == nil || *np.Props.VMSize == "" {
				err := errors.New("nodepool[].properties.VMSize is mandatory when creating node pools")
				l.Error(err, "Validation failed")
				return err
			}
		}

		if np.Action == k8smanagersv1.Create || np.Action == k8smanagersv1.Update {

			if len(np.Name) > 12 {
				err := errors.New("nodepool[].name must be 12 or fewer characters")
				l.Error(err, "Validation failed")
				return err
			}

			if k8smanagers_utils.IsLowercaseAndNumbers(ctx, np.Name) == false {
				err := errors.New("nodepool[].name must only be lowercase letter and numbers")
				l.Error(err, "Validation failed")
				return err
			}

			if k8smanagers_utils.StartsWithANumber(ctx, np.Name) {
				err := errors.New("nodepool[].name cannot start with a number")
				l.Error(err, "Validation failed")
				return err
			}

			if *np.Props.OrchestratorVersion == "" {
				err := errors.New("nodepool[].properties.OrchestratorVersion is mandatory when creating or updating node pools")
				l.Error(err, "Validation failed")
				return err
			}

			result, err := k8smanagers_utils.CompareVersions(npManager.Status.ClusterVersion, *np.Props.OrchestratorVersion)
			if err != nil {
				l.Error(err, "Version compare failed ")
				return err
			}
			if result == -1 {
				err := errors.New("the version of the node pool must not be greater than the Kubernetes version")
				l.Error(err, "Validation failed", "ClusterVersion", npManager.Status.ClusterVersion, "Requested version", np.Props.OrchestratorVersion)
				return err
			}

		} else if np.Action == k8smanagersv1.Delete {
			// Maybe check to see if the nodes have scaled to zero?
		}
	}
	return nil
}

// +kubebuilder:rbac:groups=k8smanagers.greyridge.com,resources=nodepoolmanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8smanagers.greyridge.com,resources=nodepoolmanagers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8smanagers.greyridge.com,resources=nodepoolmanagers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the NodePoolManager object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *NodePoolManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.Log
	l.Info("Enter Reconcile")

	var npManager k8smanagersv1.NodePoolManager
	if err := r.Get(ctx, req.NamespacedName, &npManager); err != nil {
		if k8serrors.IsNotFound(err) {
			// Resource was deleted, clean up and exit reconciliation
			l.Info("Exit Reconcile - No NP manager config found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	requeue := npManager.Spec.RetryOnError

	if err := r.populateStatus(ctx, &npManager); err != nil {
		return ctrl.Result{Requeue: requeue}, err
	}

	if err := r.populateSpec(ctx, &npManager); err != nil {
		return ctrl.Result{Requeue: requeue}, err
	}

	if err := r.validate(ctx, &npManager); err != nil {
		return ctrl.Result{Requeue: requeue}, err
	}

	if err := r.createNodePools(ctx, &npManager); err != nil {
		return ctrl.Result{Requeue: requeue}, err
	}

	if err := r.updateNodePools(ctx, &npManager); err != nil {
		return ctrl.Result{Requeue: requeue}, err
	}

	if err := r.deleteNodePools(ctx, &npManager); err != nil {
		return ctrl.Result{Requeue: requeue}, err
	}

	l.Info("Exit Reconcile")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePoolManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8smanagersv1.NodePoolManager{}).
		Complete(r)
}
