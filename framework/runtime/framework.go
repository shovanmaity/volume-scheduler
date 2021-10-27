package runtime

import (
	"context"
	"fmt"

	scpv1alpha1 "github.com/openebs/device-localpv/pkg/apis/openebs.io/scp/v1alpha1"
	"github.com/shovanmaity/volume-scheduler/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type Framework struct {
	//queueSortPlugins     []framework.QueueSortPlugin
	preFilterPlugins  []framework.PreFilterPlugin
	filterPlugins     []framework.FilterPlugin
	postFilterPlugins []framework.PostFilterPlugin
	preScorePlugins   []framework.PreScorePlugin
	scorePlugins      []framework.ScorePlugin
	reservePlugins    []framework.ReservePlugin
	preBindPlugins    []framework.PreBindPlugin
	bindPlugins       []framework.BindPlugin
	postBindPlugins   []framework.PostBindPlugin
	permitPlugins     []framework.PermitPlugin
}

// RunPreFilterPlugins runs set of configured PreFilter plugins. If a non-success status is
// returned, then the scheduling cycle is aborted.
func (f *Framework) RunPreFilterPlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume) (status *framework.Status) {
	for _, pl := range f.preFilterPlugins {
		status = f.runPreFilterPlugin(ctx, pl, state, volume)
		if !status.IsSuccess() {
			status.SetPluginName(pl.Name())
			if status.IsUnschedulable() {
				return status
			}
			return framework.AsStatus(fmt.Errorf("running PreFilter plugin %q: %w", pl.Name(),
				status.AsError())).WithPluginName(pl.Name())
		}
	}
	return nil
}

func (f *Framework) runPreFilterPlugin(ctx context.Context, pl framework.PreFilterPlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume) *framework.Status {
	return pl.PreFilter(ctx, state, volume)
}

// RunFilterPlugins runs the set of configured Filter plugins for volume on the given pool. If any
// of these plugins doesn't return "Success", the given pool is not suitable for the volume.
// Meanwhile, the failure message and status are set for the given pool.
func (f *Framework) RunFilterPlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, poolInfo *framework.PoolInfo) framework.PluginToStatus {
	statuses := make(framework.PluginToStatus)
	for _, pl := range f.filterPlugins {
		pluginStatus := f.runFilterPlugin(ctx, pl, state, volume, poolInfo)
		if !pluginStatus.IsSuccess() {
			if !pluginStatus.IsUnschedulable() {
				// Filter plugins are not supposed to return any status other than Success or Unschedulable.
				errStatus := framework.AsStatus(fmt.Errorf("running %q filter plugin: %w", pl.Name(),
					pluginStatus.AsError())).WithPluginName(pl.Name())
				return map[string]*framework.Status{pl.Name(): errStatus}
			}
			pluginStatus.SetPluginName(pl.Name())
			statuses[pl.Name()] = pluginStatus
		}
	}
	return statuses
}

func (f *Framework) runFilterPlugin(ctx context.Context, pl framework.FilterPlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, poolInfo *framework.PoolInfo) *framework.Status {
	return pl.Filter(ctx, state, volume, poolInfo)
}

// RunPostFilterPlugins runs the set of configured PostFilter plugins until the first
// Success or Error is met, otherwise continues to execute all plugins.
func (f *Framework) RunPostFilterPlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume /*filteredNodeStatusMap framework.NodeToStatusMap*/) (poolName string, status *framework.Status) {
	statuses := make(framework.PluginToStatus)
	for _, pl := range f.postFilterPlugins {
		r, s := f.runPostFilterPlugin(ctx, pl, state, volume /*, filteredNodeStatusMap*/)
		if s.IsSuccess() {
			return r, s
		} else if !s.IsUnschedulable() {
			// Any status other than Success or Unschedulable is Error.
			return "", framework.AsStatus(s.AsError())
		}
		statuses[pl.Name()] = s
	}

	return "", statuses.Merge()
}

func (f *Framework) runPostFilterPlugin(ctx context.Context, pl framework.PostFilterPlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume,
	/*, filteredNodeStatusMap framework.NodeToStatusMap*/) (string, *framework.Status) {
	return pl.PostFilter(ctx, state, volume /*, filteredNodeStatusMap*/)
}

// RunPreScorePlugins runs the set of configured pre-score plugins. If any
// of these plugins returns any status other than "Success", the given pod is rejected.
func (f *Framework) RunPreScorePlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pools []*scpv1alpha1.StoragePool) (status *framework.Status) {
	for _, pl := range f.preScorePlugins {
		status = f.runPreScorePlugin(ctx, pl, state, volume, pools)
		if !status.IsSuccess() {
			return framework.AsStatus(fmt.Errorf("running PreScore plugin %q: %w", pl.Name(), status.AsError()))
		}
	}
	return nil
}

func (f *Framework) runPreScorePlugin(ctx context.Context, pl framework.PreScorePlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume,
	pools []*scpv1alpha1.StoragePool) *framework.Status {
	return pl.PreScore(ctx, state, volume, pools)
}

// RunScorePlugins runs the set of configured scoring plugins. It returns a list that
// stores for each scoring plugin name the corresponding NodeScoreList(s).
// It also returns *Status, which is set to non-success if any of the plugins returns
// a non-success status.
func (f *Framework) RunScorePlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pools []*scpv1alpha1.StoragePool) (
	ps framework.PluginToPoolScores, status *framework.Status) {
	pluginToPoolScores := make(framework.PluginToPoolScores, len(f.scorePlugins))
	for _, pl := range f.scorePlugins {
		pluginToPoolScores[pl.Name()] = make(framework.PoolScoreList, len(pools))
	}
	/*
		ctx, cancel := context.WithCancel(ctx)
			errCh := parallelize.NewErrorChannel()

			// Run Score method for each node in parallel.
			f.Parallelizer().Until(ctx, len(nodes), func(index int) {
				for _, pl := range f.scorePlugins {
					nodeName := nodes[index].Name
					s, status := f.runScorePlugin(ctx, pl, state, pod, nodeName)
					if !status.IsSuccess() {
						err := fmt.Errorf("plugin %q failed with: %w", pl.Name(), status.AsError())
						errCh.SendErrorWithCancel(err, cancel)
						return
					}
					pluginToNodeScores[pl.Name()][index] = framework.NodeScore{
						Name:  nodeName,
						Score: s,
					}
				}
			})
			if err := errCh.ReceiveError(); err != nil {
				return nil, framework.AsStatus(fmt.Errorf("running Score plugins: %w", err))
			}

			// Run NormalizeScore method for each ScorePlugin in parallel.
			f.Parallelizer().Until(ctx, len(f.scorePlugins), func(index int) {
				pl := f.scorePlugins[index]
				nodeScoreList := pluginToNodeScores[pl.Name()]
				if pl.ScoreExtensions() == nil {
					return
				}
				status := f.runScoreExtension(ctx, pl, state, pod, nodeScoreList)
				if !status.IsSuccess() {
					err := fmt.Errorf("plugin %q failed with: %w", pl.Name(), status.AsError())
					errCh.SendErrorWithCancel(err, cancel)
					return
				}
			})
			if err := errCh.ReceiveError(); err != nil {
				return nil, framework.AsStatus(fmt.Errorf("running Normalize on Score plugins: %w", err))
			}

			// Apply score defaultWeights for each ScorePlugin in parallel.
			f.Parallelizer().Until(ctx, len(f.scorePlugins), func(index int) {
				pl := f.scorePlugins[index]
				// Score plugins' weight has been checked when they are initialized.
				weight := f.scorePluginWeight[pl.Name()]
				nodeScoreList := pluginToNodeScores[pl.Name()]

				for i, nodeScore := range nodeScoreList {
					// return error if score plugin returns invalid score.
					if nodeScore.Score > framework.MaxNodeScore || nodeScore.Score < framework.MinNodeScore {
						err := fmt.Errorf("plugin %q returns an invalid score %v, it should in the range of [%v, %v] after normalizing", pl.Name(), nodeScore.Score, framework.MinNodeScore, framework.MaxNodeScore)
						errCh.SendErrorWithCancel(err, cancel)
						return
					}
					nodeScoreList[i].Score = nodeScore.Score * int64(weight)
				}
			})
			if err := errCh.ReceiveError(); err != nil {
				return nil, framework.AsStatus(fmt.Errorf("applying score defaultWeights on Score plugins: %w", err))
			}
	*/
	return pluginToPoolScores, nil
}

func (f *Framework) runScorePlugin(ctx context.Context, pl framework.ScorePlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, pool,
	cohort *corev1.ObjectReference) (int64, *framework.Status) {
	return pl.Score(ctx, state, volume, pool, cohort)
}

/*
func (f *Framework) runScoreExtension(ctx context.Context, pl framework.ScorePlugin, state *framework.CycleState, pod *v1.Pod, nodeScoreList framework.NodeScoreList) *framework.Status {
	return pl.ScoreExtensions().NormalizeScore(ctx, state, pod, nodeScoreList)
}
*/

// RunReservePluginsReserve runs the Reserve method in the set of configured
// reserve plugins. If any of these plugins returns an error, it does not
// continue running the remaining ones and returns the error. In such a case,
// the pod will not be scheduled and the caller will be expected to call
// RunReservePluginsUnreserve.
func (f *Framework) RunReservePluginsReserve(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) (status *framework.Status) {
	for _, pl := range f.reservePlugins {
		status = f.runReservePluginReserve(ctx, pl, state, volume, pool, cohort)
		if !status.IsSuccess() {
			err := status.AsError()
			klog.ErrorS(err, "Failed running Reserve plugin", "plugin", pl.Name(), "volume", klog.KObj(volume))
			return framework.AsStatus(fmt.Errorf("running Reserve plugin %q: %w", pl.Name(), err))
		}
	}
	return nil
}

func (f *Framework) runReservePluginReserve(ctx context.Context, pl framework.ReservePlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, pool,
	cohort *corev1.ObjectReference) *framework.Status {
	return pl.Reserve(ctx, state, volume, pool, cohort)
}

// RunReservePluginsUnreserve runs the Unreserve method in the set of
// configured reserve plugins.
func (f *Framework) RunReservePluginsUnreserve(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) {
	// Execute the Unreserve operation of each reserve plugin in the
	// *reverse* order in which the Reserve operation was executed.
	for i := len(f.reservePlugins) - 1; i >= 0; i-- {
		f.runReservePluginUnreserve(ctx, f.reservePlugins[i], state, volume, pool, cohort)
	}
}

func (f *Framework) runReservePluginUnreserve(ctx context.Context, pl framework.ReservePlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) {
	pl.Unreserve(ctx, state, volume, pool, cohort)
}

// RunPreBindPlugins runs the set of configured prebind plugins. It returns a
// failure (bool) if any of the plugins returns an error. It also returns an
// error containing the rejection message or the error occurred in the plugin.
func (f *Framework) RunPreBindPlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) (status *framework.Status) {
	for _, pl := range f.preBindPlugins {
		status = f.runPreBindPlugin(ctx, pl, state, volume, pool, cohort)
		if !status.IsSuccess() {
			err := status.AsError()
			klog.ErrorS(err, "Failed running PreBind plugin", "plugin", pl.Name(), "volume", klog.KObj(volume))
			return framework.AsStatus(fmt.Errorf("running PreBind plugin %q: %w", pl.Name(), err))
		}
	}
	return nil
}

func (f *Framework) runPreBindPlugin(ctx context.Context, pl framework.PreBindPlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, pool,
	cohort *corev1.ObjectReference) *framework.Status {
	return pl.PreBind(ctx, state, volume, pool, cohort)
}

// RunBindPlugins runs the set of configured bind plugins until one returns a non `Skip` status.
func (f *Framework) RunBindPlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) (status *framework.Status) {
	if len(f.bindPlugins) == 0 {
		return framework.NewStatus(framework.Skip, "")
	}
	for _, bp := range f.bindPlugins {
		status = f.runBindPlugin(ctx, bp, state, volume, pool, cohort)
		if status != nil && status.Code() == framework.Skip {
			continue
		}
		if !status.IsSuccess() {
			err := status.AsError()
			klog.ErrorS(err, "Failed running Bind plugin", "plugin", bp.Name(), "volume", klog.KObj(volume))
			return framework.AsStatus(fmt.Errorf("running Bind plugin %q: %w", bp.Name(), err))
		}
		return status
	}
	return status
}

func (f *Framework) runBindPlugin(ctx context.Context, bp framework.BindPlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, pool,
	cohort *corev1.ObjectReference) *framework.Status {
	return bp.Bind(ctx, state, volume, pool, cohort)
}

// RunPostBindPlugins runs the set of configured postbind plugins.
func (f *Framework) RunPostBindPlugins(ctx context.Context, state *framework.CycleState,
	volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) {
	for _, pl := range f.postBindPlugins {
		f.runPostBindPlugin(ctx, pl, state, volume, pool, cohort)
	}
}

func (f *Framework) runPostBindPlugin(ctx context.Context, pl framework.PostBindPlugin,
	state *framework.CycleState, volume *scpv1alpha1.StorageVolume, pool, cohort *corev1.ObjectReference) {
	pl.PostBind(ctx, state, volume, pool, cohort)
}
