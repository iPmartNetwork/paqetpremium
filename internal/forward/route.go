package forward

import "github.com/paqetpremium/paqetpremium/internal/tunnelpool"

// RouteFn resolves a tunnel opener for optional upstream binding.
type RouteFn func(bind string) tunnelpool.Opener
