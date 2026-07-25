// Registry implementation for Hito 5 slice 5.2.
//
// Registry holds the set of detectors known to a royo-learn process
// and exposes a stable lookup by Kind(). The orchestrator (slice 5.3)
// and the acceptance scenario (slice 5.4) consume this; tests build
// their own minimal Registry without depending on a global.
//
// Thread safety: Registry is not safe for concurrent registration.
// The orchestrator registers all detectors at startup and only reads
// afterward. Concurrent registration is not required at this stage
// of Hito 5 and can be added with a sync.RWMutex when slice 8 (the
// jobs engine) needs hot reload.

package detectors

import "fmt"

// Registry is the orchestrator-facing detector lookup table.
type Registry struct {
	detectors map[string]Detector
	order     []string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{detectors: make(map[string]Detector)}
}

// Register adds a detector to the registry. Returns an error if d is
// nil, d.Kind() is empty, d.Version() is empty, or a detector with
// the same Kind has already been registered.
//
// Misconfiguration is reported at registration time so the
// orchestrator fails fast at startup rather than at the first
// observation. The first successful registration wins: a duplicate
// rejection does not overwrite the existing detector.
func (r *Registry) Register(d Detector) error {
	if d == nil {
		return fmt.Errorf("detectors: registry: nil detector")
	}
	kind := d.Kind()
	if kind == "" {
		return fmt.Errorf("detectors: registry: detector has empty Kind()")
	}
	if d.Version() == "" {
		return fmt.Errorf("detectors: registry: detector %q has empty Version()", kind)
	}
	if _, exists := r.detectors[kind]; exists {
		return fmt.Errorf("detectors: registry: detector %q already registered", kind)
	}
	r.detectors[kind] = d
	r.order = append(r.order, kind)
	return nil
}

// Get returns the detector registered under kind, or (nil, false) if
// none is registered. The returned detector is the one the registry
// stored at registration time; callers must not mutate it.
func (r *Registry) Get(kind string) (Detector, bool) {
	d, ok := r.detectors[kind]
	return d, ok
}

// Kinds returns the registered kinds in insertion order. The slice
// is a defensive copy: mutating it does not affect the registry and
// the orchestrator can safely pass the result to JSON encoders or
// sort routines without side effects on the registry.
func (r *Registry) Kinds() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Len returns the number of registered detectors.
func (r *Registry) Len() int {
	return len(r.detectors)
}
