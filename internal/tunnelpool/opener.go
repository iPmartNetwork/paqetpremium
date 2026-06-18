package tunnelpool

import "github.com/xtaci/smux"

// Opener opens multiplexed streams through an active tunnel pool.
type Opener interface {
	OpenTCP(target string) (*smux.Stream, error)
	OpenUDP(localAddr, target string) (*smux.Stream, bool, uint64, error)
	CloseUDP(key uint64)
}
