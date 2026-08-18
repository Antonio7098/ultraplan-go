package web

import (
	"testing"
	"time"
)

func TestServerPolicyDefaultsAndFailClosedValidation(t *testing.T) {
	if err := ValidateServerPolicy(DefaultServerPolicy()); err != nil {
		t.Fatal(err)
	}
	cases := []func(*ServerPolicy){
		func(policy *ServerPolicy) { policy.MaxInFlight = 0 },
		func(policy *ServerPolicy) { policy.SSEHeartbeat = policy.MaxStreamLifetime },
		func(policy *ServerPolicy) { policy.PreparationTTL = policy.TerminalRetention },
		func(policy *ServerPolicy) { policy.MaxEncodedEventBytes = policy.MaxEventBytesPerOperation + 1 },
		func(policy *ServerPolicy) { policy.SubscriberQueueSize = policy.MaxEventsPerOperation + 1 },
		func(policy *ServerPolicy) { policy.ReadHeaderTimeout = policy.ReadTimeout + time.Second },
	}
	for index, mutate := range cases {
		policy := DefaultServerPolicy()
		mutate(&policy)
		if err := ValidateServerPolicy(policy); err == nil {
			t.Errorf("case %d accepted incoherent policy: %+v", index, policy)
		}
	}
}
