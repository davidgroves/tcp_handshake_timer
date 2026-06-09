package ebpf

import (
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type clsactLink struct {
	filter *netlink.BpfFilter
}

func (l *clsactLink) Close() error {
	if l.filter == nil {
		return nil
	}
	return netlink.FilterDel(l.filter)
}

func attachClsactProgram(ifindex int, prog *ebpf.Program, ingress bool) (*clsactLink, error) {
	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: ifindex,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := netlink.QdiscReplace(qdisc); err != nil {
		return nil, fmt.Errorf("ensure clsact qdisc: %w", err)
	}

	parent := netlink.HANDLE_MIN_EGRESS
	if ingress {
		parent = netlink.HANDLE_MIN_INGRESS
	}

	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ifindex,
			Parent:    uint32(parent),
			Priority:  1,
			Protocol:  unix.ETH_P_ALL,
		},
		Fd:           prog.FD(),
		DirectAction: true,
	}
	if err := netlink.FilterReplace(filter); err != nil {
		return nil, fmt.Errorf("attach bpf filter: %w", err)
	}
	return &clsactLink{filter: filter}, nil
}
