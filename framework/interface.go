package framework

import (
	"context"
	"time"

	scpv1alpha1 "github.com/openebs/device-localpv/pkg/apis/openebs.io/scp/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type VolumeInfo struct {
	Volume *scpv1alpha1.StorageVolume
}
type PoolInfo struct {
	Pool *scpv1alpha1.StoragePool
}

// Plugin is the parent type for all the scheduling framework plugins.
type Plugin interface {
	Name() string
}

// PreFilterPlugin is an interface that must be implemented by "PreFilter" plugins.
// These plugins are called at the beginning of the scheduling cycle.
type PreFilterPlugin interface {
	Plugin
	// PreFilter is called at the beginning of the scheduling cycle. All PreFilter plugins
	// must return success or the pod will be rejected.
	PreFilter(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume) *Status
	// PreFilterExtensions returns a PreFilterExtensions interface if the plugin implements one,
	// or nil if it does not. A Pre-filter plugin can provide extensions to incrementally modify
	// its pre-processed info. The framework guarantees that the extensions AddVolume or
	// RemoveVolume will only be called after PreFilter, possibly on a cloned CycleState, and
	// may call those functions more than once before calling Filter again on a specific node.
	PreFilterExtensions() PreFilterExtensions
}

// PreFilterExtensions is an interface that is included in plugins that allow specifying callbacks
// to make incremental updates to its supposedly pre-calculated state.
type PreFilterExtensions interface {
	// AddVolume is called by the framework while trying to evaluate the impact of adding
	// volumeToAdd to the pool while scheduling volumeToSchedule.
	AddVolume(ctx context.Context, state *CycleState, volumeToSchedule *scpv1alpha1.StorageVolume,
		volumeInfoToAdd *VolumeInfo, poolInfo *PoolInfo) *Status
	// RemoveVolume is called by the framework while trying to evaluate the impact of removing
	// volumeToRemove from the pool while scheduling volumeToSchedule.
	RemoveVolume(ctx context.Context, state *CycleState, volumeToSchedule *scpv1alpha1.StorageVolume,
		volumeInfoToRemove *VolumeInfo, poolInfo *PoolInfo) *Status
}

// FilterPlugin is an interface for Filter plugins. These plugins are called at the filter
// extension point for filtering out pools in which we can not create the volume. This concept
// used to be called 'predicate' in the original scheduler. These plugins should return "Success",
// "Unschedulable" or "Error" in Status.code. However, the scheduler accepts other valid codes as
// well. Anything other than "Success" will lead to exclusion of the given pool from the volume.
type FilterPlugin interface {
	Plugin
	// Filter is called by the scheduling framework. All FilterPlugins should return "Success" to
	// declare that the given pool fits the volume. If Filter doesn't return "Success", it will
	// return "Unschedulable", "UnschedulableAndUnresolvable" or "Error". For the pool being
	// evaluated, Filter plugins should look at the passed poolInfo reference for this particular
	// pools's information.
	Filter(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		poolInfo *PoolInfo) *Status
}

// PostFilterPlugin is an interface for "PostFilter" plugins. These plugins are called after a pod
// cannot be scheduled.
type PostFilterPlugin interface {
	Plugin
	// PostFilter is called by the scheduling framework.
	// A PostFilter plugin should return one of the following statuses:
	// - Unschedulable: the plugin gets executed successfully but the pod cannot be made schedulable.
	// - Success: the plugin gets executed successfully and the pod can be made schedulable.
	// - Error: the plugin aborts due to some internal error.
	//
	// Informational plugins should be configured ahead of other ones, and always return Unschedulable
	// status. Optionally, a non-nil PostFilterResult may be returned along with a Success status.
	// For example, a preemption plugin may choose to return nominatedNodeName, so that framework
	// can reuse that to update the preemptor pod's .spec.status.nominatedNodeName field.
	PostFilter(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		filteredPoolStatusMap PoolToStatusMap) (string, *Status)
}

// PreScorePlugin is an interface for "PreScore" plugin. PreScore is an informational extension
// point. Plugins will be called with a list of pools that passed the filtering phase. A plugin
// may use this data to update internal state or to generate logs/metrics.
type PreScorePlugin interface {
	Plugin
	// PreScore is called by the scheduling framework after a list of pools passed the filtering
	// phase. All prescore plugins must return success or the pod will be rejected.
	PreScore(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pools []*scpv1alpha1.StoragePool) *Status
}

// ScorePlugin is an interface that must be implemented by "Score" plugins to rank pools that passed
// the filtering phase.
type ScorePlugin interface {
	Plugin
	// Score is called on each filtered pool. It must return success and an integer indicating the
	// rank of the pool. All scoring plugins must return success or the volume will be rejected.
	Score(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool, cohort *corev1.ObjectReference) (int64, *Status)

	// ScoreExtensions returns a ScoreExtensions interface if it implements one, or nil if does not.
	ScoreExtensions() ScoreExtensions
}

// ScoreExtensions is an interface for Score extended functionality.
type ScoreExtensions interface {
	// NormalizeScore is called for all pools scores produced by the same plugin's "Score"
	// method. A successful run of NormalizeScore will update the scores list and return
	// a success status.
	//NormalizeScore(ctx context.Context, state *CycleState, p *v1.Pod, scores NodeScoreList) *Status
}

// ReservePlugin is an interface for plugins with Reserve and Unreserve methods. These are meant
// to update the state of the plugin. This concept used to be called 'assume' in the original
// scheduler. These plugins should return only Success or Error in Status.code. However, the
// scheduler accepts other valid codes as well. Anything other than Success will lead to rejection
// of the pod.
type ReservePlugin interface {
	Plugin
	// Reserve is called by the scheduling framework when the scheduler cache is updated. If this
	// method returns a failed Status, the scheduler will call the Unreserve method for all enabled
	// ReservePlugins.
	Reserve(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool *corev1.ObjectReference, cohort *corev1.ObjectReference) *Status
	// Unreserve is called by the scheduling framework when a reserved volume was rejected, an
	// error occurred during reservation of subsequent plugins, or in a later phase. The Unreserve
	// method implementation must be idempotent and may be called by the scheduler even if the
	// corresponding Reserve method for the same plugin was not called.
	Unreserve(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool *corev1.ObjectReference, cohort *corev1.ObjectReference)
}

// PreBindPlugin is an interface that must be implemented by "PreBind" plugins. These plugins are
// called before a volume being scheduled.
type PreBindPlugin interface {
	Plugin
	// PreBind is called before binding a volume. All prebind plugins must return success or the
	// volume will be rejected and won't be sent for binding.
	PreBind(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool *corev1.ObjectReference, cohort *corev1.ObjectReference) *Status
}

// PostBindPlugin is an interface that must be implemented by "PostBind" plugins. These plugins are
// called after a volume is successfully bound to a pool.
type PostBindPlugin interface {
	Plugin
	// PostBind is called after a volume is successfully bound. These plugins are informational.
	// A common application of this extension point is for cleaning up. If a plugin needs to
	// clean-up its state after a volume is scheduled and bound, PostBind is the extension point
	// that it should register.
	PostBind(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool *corev1.ObjectReference, cohort *corev1.ObjectReference)
}

// PermitPlugin is an interface that must be implemented by "Permit" plugins. These plugins are
// called before a volume is bound to a pool.
type PermitPlugin interface {
	Plugin
	// Permit is called before binding a volume (and before prebind plugins). Permit plugins are
	// used to prevent or delay the binding of a volume. A permit plugin must return success or
	// wait with timeout duration, or the volume will be rejected. The volume will also be rejected
	// if the wait timeout or the volume is rejected while waiting. Note that if the plugin returns
	// "wait", the framework will wait only after running the remaining plugins given that no other
	// plugin rejects the volume.
	Permit(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool *corev1.ObjectReference, cohort *corev1.ObjectReference) (*Status, time.Duration)
}

// BindPlugin is an interface that must be implemented by "Bind" plugins. Bind plugins are used to
// bind a volume to a pool.
type BindPlugin interface {
	Plugin
	// Bind plugins will not be called until all pre-bind plugins have completed. Each bind plugin
	// is called in the configured order. A bind plugin may choose whether or not to handle the
	// given volume. If a bind plugin chooses to handle a volume, the remaining bind plugins are
	// skipped. When a bind plugin does not handle a volume, it must return Skip in its Status
	// code. If a bind plugin returns an Error, the volume is rejected and will not be bound.
	Bind(ctx context.Context, state *CycleState, volume *scpv1alpha1.StorageVolume,
		pool *corev1.ObjectReference, cohort *corev1.ObjectReference) *Status
}
