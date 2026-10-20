package permission

// agentScopedService evaluates a per-agent Policy before falling back to the
// wrapped service. Requests the policy is silent about keep the default
// interactive flow.
type agentScopedService struct {
	// Service is the wrapped base service. Every method but Request is
	// inherited from it.
	Service
	policy Policy
}

// NewAgentScoped wraps base with an agent's permission policy. When the policy
// has no rules base is returned unchanged, so agents without a permissions
// block share the global behaviour.
//
// A deny is final: it rejects without prompting, even if the session was
// auto-approved, because an agent that may not use a tool is not something the
// user can override per request.
func NewAgentScoped(base Service, policy Policy) Service {
	if base == nil || !policy.HasRules {
		return base
	}
	return &agentScopedService{Service: base, policy: policy}
}

func (s *agentScopedService) Request(opts CreatePermissionRequest) bool {
	action, matched := s.policy.Decide(opts)
	if !matched {
		return s.Service.Request(opts)
	}
	switch action {
	case ActionDeny:
		return false
	case ActionAllow:
		return true
	default:
		return s.Service.Request(opts)
	}
}
