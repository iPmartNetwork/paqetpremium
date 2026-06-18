package netutil

import "sync/atomic"

type FlagCycle struct {
	items []TCPFlagSet
	idx   uint32
}

func NewFlagCycle(items []TCPFlagSet) *FlagCycle {
	if len(items) == 0 {
		items = []TCPFlagSet{{PSH: true, ACK: true}}
	}
	return &FlagCycle{items: items}
}

func (c *FlagCycle) Next() TCPFlagSet {
	i := atomic.AddUint32(&c.idx, 1) - 1
	return c.items[int(i)%len(c.items)]
}
