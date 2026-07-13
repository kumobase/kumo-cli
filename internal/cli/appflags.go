package cli

import (
	"github.com/spf13/pflag"

	"github.com/kumobase/kumo-go/types"
)

// autoscaleFlags holds the horizontal-autoscaling flags shared by app create
// (registry + git-build) and update.
type autoscaleFlags struct {
	enabled   bool
	min, max  int
	cpuTarget int
	memTarget int
}

func addAutoscaleFlags(f *pflag.FlagSet, a *autoscaleFlags) {
	f.BoolVar(&a.enabled, "autoscale", false, "enable horizontal autoscaling")
	f.IntVar(&a.min, "min-replicas", 0, "minimum replicas (with --autoscale)")
	f.IntVar(&a.max, "max-replicas", 0, "maximum replicas (with --autoscale)")
	f.IntVar(&a.cpuTarget, "cpu-target", 0, "target CPU utilization percent (with --autoscale)")
	f.IntVar(&a.memTarget, "mem-target", 0, "target memory utilization percent (with --autoscale)")
}

func autoscaleChanged(f *pflag.FlagSet) bool {
	return f.Changed("autoscale") || f.Changed("min-replicas") || f.Changed("max-replicas") ||
		f.Changed("cpu-target") || f.Changed("mem-target")
}

// build returns an AutoscalingConfig when any autoscaling flag was set on f,
// else nil (leave the field untouched).
func (a *autoscaleFlags) build(f *pflag.FlagSet) *types.AutoscalingConfig {
	if !autoscaleChanged(f) {
		return nil
	}
	cfg := &types.AutoscalingConfig{Enabled: a.enabled, MinReplicas: a.min, MaxReplicas: a.max}
	if f.Changed("cpu-target") {
		v := a.cpuTarget
		cfg.CPUTargetPercentage = &v
	}
	if f.Changed("mem-target") {
		v := a.memTarget
		cfg.MemoryTargetPercentage = &v
	}
	return cfg
}

// healthCheckFlags holds the container health-check flags shared by app create
// and update.
type healthCheckFlags struct {
	typ  string
	path string
	port uint16
}

func addHealthCheckFlags(f *pflag.FlagSet, h *healthCheckFlags) {
	f.StringVar(&h.typ, "health-check-type", "", "health check type (e.g. http)")
	f.StringVar(&h.path, "health-check-path", "", "health check path (e.g. /healthz)")
	f.Uint16Var(&h.port, "health-check-port", 0, "health check port")
}

func (h *healthCheckFlags) build(f *pflag.FlagSet) *types.HealthCheck {
	if !f.Changed("health-check-type") && !f.Changed("health-check-path") && !f.Changed("health-check-port") {
		return nil
	}
	return &types.HealthCheck{Type: h.typ, Path: h.path, Port: h.port}
}
