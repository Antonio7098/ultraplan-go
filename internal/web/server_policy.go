package web

import (
	"errors"
	"time"
)

// ServerPolicy is the immutable effective set of local-web safety bounds. The
// current compatibility contract exposes only built-in values; operators can
// select the listen address and browser launch behavior, but cannot weaken
// these caps through workspace configuration or environment variables.
type ServerPolicy struct {
	ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout, ShutdownTimeout time.Duration
	MaxInFlight, MaxBodyBytes, MaxRequestTarget, MaxIdentifierBytes            int
	MaxActiveOperations, MaxPreparations                                       int
	PreparationTTL, TerminalRetention, SSEHeartbeat, MaxStreamLifetime         time.Duration
	MaxEventsPerOperation, MaxEventBytesPerOperation, MaxEncodedEventBytes     int
	MaxTerminalResultBytes, MaxSubscribersPerOperation, MaxConcurrentStreams   int
	SubscriberQueueSize                                                        int
}

func DefaultServerPolicy() ServerPolicy {
	return ServerPolicy{
		ReadHeaderTimeout: ReadHeaderTimeout, ReadTimeout: ReadTimeout, WriteTimeout: WriteTimeout, IdleTimeout: IdleTimeout, ShutdownTimeout: ShutdownTimeout,
		MaxInFlight: MaxInFlight, MaxBodyBytes: MaxBodyBytes, MaxRequestTarget: MaxRequestTarget, MaxIdentifierBytes: MaxIdentifierBytes,
		MaxActiveOperations: MaxActiveOperations, MaxPreparations: MaxPreparations, PreparationTTL: PreparationTTL,
		TerminalRetention: TerminalRetention, SSEHeartbeat: SSEHeartbeat, MaxStreamLifetime: MaxStreamLifetime,
		MaxEventsPerOperation: MaxEventsPerOperation, MaxEventBytesPerOperation: MaxEventBytesPerOperation, MaxEncodedEventBytes: MaxEncodedEventBytes,
		MaxTerminalResultBytes: MaxTerminalResultBytes, MaxSubscribersPerOperation: MaxSubscribersPerOperation,
		MaxConcurrentStreams: MaxConcurrentStreams, SubscriberQueueSize: SubscriberQueueSize,
	}
}

func ValidateServerPolicy(policy ServerPolicy) error {
	durations := []time.Duration{policy.ReadHeaderTimeout, policy.ReadTimeout, policy.WriteTimeout, policy.IdleTimeout, policy.ShutdownTimeout, policy.PreparationTTL, policy.TerminalRetention, policy.SSEHeartbeat, policy.MaxStreamLifetime}
	for _, value := range durations {
		if value <= 0 {
			return errors.New("all local-web durations must be positive")
		}
	}
	limits := []int{policy.MaxInFlight, policy.MaxBodyBytes, policy.MaxRequestTarget, policy.MaxIdentifierBytes, policy.MaxActiveOperations, policy.MaxPreparations, policy.MaxEventsPerOperation, policy.MaxEventBytesPerOperation, policy.MaxEncodedEventBytes, policy.MaxTerminalResultBytes, policy.MaxSubscribersPerOperation, policy.MaxConcurrentStreams, policy.SubscriberQueueSize}
	for _, value := range limits {
		if value <= 0 {
			return errors.New("all local-web resource limits must be positive")
		}
	}
	if policy.ReadHeaderTimeout > policy.ReadTimeout || policy.SSEHeartbeat >= policy.MaxStreamLifetime ||
		policy.PreparationTTL >= policy.TerminalRetention || policy.MaxEncodedEventBytes > policy.MaxEventBytesPerOperation ||
		policy.SubscriberQueueSize > policy.MaxEventsPerOperation || policy.MaxSubscribersPerOperation > policy.MaxConcurrentStreams {
		return errors.New("local-web resource limits are incoherent")
	}
	return nil
}
