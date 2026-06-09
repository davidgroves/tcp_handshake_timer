package ebpf

import (
	"fmt"
	"net"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"

	"github.com/dwg/tcp_handshake_timer/internal/config"
	"github.com/dwg/tcp_handshake_timer/internal/measurement"
)

type AttachMode string

const (
	AttachModeTCX    AttachMode = "tcx"
	AttachModeClsact AttachMode = "clsact"
)

type Loader struct {
	objs   HandshakeObjects
	links  []closable
	mode   AttachMode
	Events *ebpf.Map
	Stats  *ebpf.Map
}

type closable interface {
	Close() error
}

func NewLoader(cfg config.Config) (*Loader, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("remove memlock: %w", err)
	}

	objs := HandshakeObjects{}
	if err := LoadHandshakeObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("load bpf objects: %w", err)
	}

	l := &Loader{
		objs:   objs,
		Events: objs.Events,
		Stats:  objs.Stats,
	}

	if err := l.configureMaps(cfg); err != nil {
		objs.Close()
		return nil, err
	}

	mode, err := l.attachInterfaces(cfg.Interfaces)
	if err != nil {
		objs.Close()
		return nil, err
	}
	l.mode = mode
	return l, nil
}

func (l *Loader) configureMaps(cfg config.Config) error {
	var cfgVal HandshakeThtimerConfig
	cfgVal.SampleRatePct = cfg.SampleRatePct
	if cfg.PortFilterEnabled() {
		cfgVal.PortFilterEnabled = 1
	}
	cfgVal.PrefixV4 = uint32(cfg.SubnetPrefixV4)
	cfgVal.PrefixV6 = uint32(cfg.SubnetPrefixV6)

	key := uint32(0)
	if err := l.objs.ConfigMap.Put(key, cfgVal); err != nil {
		return fmt.Errorf("update config map: %w", err)
	}

	for _, port := range cfg.Ports {
		val := uint8(1)
		if err := l.objs.Ports.Put(port, val); err != nil {
			return fmt.Errorf("update ports map for %d: %w", port, err)
		}
	}
	return nil
}

func (l *Loader) attachInterfaces(names []string) (AttachMode, error) {
	var firstErr error
	for _, name := range names {
		iface, err := net.InterfaceByName(name)
		if err != nil {
			return "", fmt.Errorf("lookup interface %q: %w", name, err)
		}
		if err := l.attachInterface(iface); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}
	if len(l.links) == 0 {
		if firstErr != nil {
			return "", firstErr
		}
		return "", fmt.Errorf("no interfaces attached")
	}
	if l.mode == "" {
		l.mode = AttachModeTCX
	}
	return l.mode, nil
}

func (l *Loader) attachInterface(iface *net.Interface) error {
	ingress, err := link.AttachTCX(link.TCXOptions{
		Interface: iface.Index,
		Program:   l.objs.ThtimerIngress,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err == nil {
		egress, err2 := link.AttachTCX(link.TCXOptions{
			Interface: iface.Index,
			Program:   l.objs.ThtimerEgress,
			Attach:    ebpf.AttachTCXEgress,
		})
		if err2 != nil {
			ingress.Close()
			return l.attachClsact(iface)
		}
		l.links = append(l.links, ingress, egress)
		return nil
	}
	return l.attachClsact(iface)
}

func (l *Loader) attachClsact(iface *net.Interface) error {
	ingress, err := attachClsactProgram(iface.Index, l.objs.ThtimerIngress, true)
	if err != nil {
		return fmt.Errorf("attach clsact ingress on %s: %w", iface.Name, err)
	}
	egress, err := attachClsactProgram(iface.Index, l.objs.ThtimerEgress, false)
	if err != nil {
		_ = ingress.Close()
		return fmt.Errorf("attach clsact egress on %s: %w", iface.Name, err)
	}
	l.links = append(l.links, ingress, egress)
	l.mode = AttachModeClsact
	return nil
}

func (l *Loader) ReadStats() ([measurement.StatMax]uint64, error) {
	var totals [measurement.StatMax]uint64
	for i := uint32(0); i < measurement.StatMax; i++ {
		var perCPU []uint64
		if err := l.objs.Stats.Lookup(i, &perCPU); err != nil {
			return totals, fmt.Errorf("lookup stat %d: %w", i, err)
		}
		for _, v := range perCPU {
			totals[i] += v
		}
	}
	return totals, nil
}

func (l *Loader) Close() error {
	var first error
	for _, ln := range l.links {
		if err := ln.Close(); err != nil && first == nil {
			first = err
		}
	}
	l.objs.Close()
	return first
}

func (l *Loader) Mode() AttachMode {
	return l.mode
}
