package yggstack

import (
	"context"
	"os"
	"time"
)

// // // // // // // // // //

// startComponents recreates all subsystems using the stored nodeConfig.
// Called from LPM when waking up.
func (o *Obj) startComponents(cfg ConfigObj) (retErr error) {
	o.componentsMu.Lock()
	defer o.componentsMu.Unlock()

	// New generation context for component goroutines
	o.componentsCtx, o.componentsCancel = context.WithCancel(o.ctx)

	// Channel to signal SOCKS readiness on wake
	if cfg.SocksAddr != "" {
		o.socksReadyCh = make(chan struct{})
	}

	nodeCfg := o.nodeConfig
	log := o.logger

	defer func() {
		if retErr != nil {
			o.netstackPtr.Store(nil)
			o.rollbackComponents()
			o.componentsCancel()
			o.componentsWg.Wait()
			o.componentsCancel = nil
			o.componentsCtx = nil
		}
	}()

	if err := o.initCore(nodeCfg, log); err != nil {
		return err
	}
	if err := o.initAdmin(nodeCfg, log); err != nil {
		return err
	}
	if err := o.initMulticast(cfg, nodeCfg); err != nil {
		return err
	}
	if err := o.initNetworking(cfg, log); err != nil {
		return err
	}

	return nil
}

// stopComponents shuts down subsystems without finalizing Obj.
// Called from Close() and from LPM when entering sleep.
func (o *Obj) stopComponents() {
	o.componentsMu.Lock()
	defer o.componentsMu.Unlock()

	if o.componentsCancel != nil {
		o.componentsCancel()
	}

	if o.socksListener != nil {
		_ = o.socksListener.Close()
		if o.socksIsUnix {
			_ = os.Remove(o.socksAddr)
			o.logger.Infof("Stopped SOCKS5 UNIX socket listener")
		} else {
			o.logger.Infof("Stopped SOCKS5 TCP listener")
		}
		o.socksListener = nil
	}
	o.closersMu.Lock()
	for _, c := range o.closers {
		_ = c.Close()
	}
	o.closers = nil
	o.closersMu.Unlock()

	o.componentsWg.Wait()

	// Peer monitor depends on Core, stop it before Core.Stop()
	if o.peerMonitor != nil {
		o.peerMonitor.Cancel()
		o.peerMonitor = nil
	}
	// Atomically swap out the netstack — stops new Dial/Listen calls from using it.
	if ns := o.netstackPtr.Swap(nil); ns != nil {
		ns.Close()
	}
	if o.Multicast != nil {
		_ = o.Multicast.Stop()
		o.Multicast = nil
	}
	if o.Admin != nil {
		_ = o.Admin.Stop()
		o.Admin = nil
	}
	o.stopCoreWithTimeout()
	o.componentsCancel = nil
	o.componentsCtx = nil
}

// stopCoreWithTimeout stops Core with a time limit.
// When timeout == 0 — waits indefinitely (backward-compatible).
func (o *Obj) stopCoreWithTimeout() {
	if o.Core == nil {
		return
	}
	if o.coreStopTimeout == 0 {
		o.Core.Stop()
		o.Core = nil
		return
	}
	done := make(chan struct{})
	go func() {
		o.Core.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(o.coreStopTimeout):
		o.logger.Warnf("core.Stop() timed out after %s, forcing shutdown", o.coreStopTimeout)
	}
	o.Core = nil
}

// rollbackComponents stops partially initialized subsystems.
func (o *Obj) rollbackComponents() {
	if o.peerMonitor != nil {
		o.peerMonitor.Cancel()
		o.peerMonitor = nil
	}
	if o.socksListener != nil {
		_ = o.socksListener.Close()
		o.socksListener = nil
	}
	if ns := o.netstackPtr.Swap(nil); ns != nil {
		ns.Close()
	}
	if o.Multicast != nil {
		_ = o.Multicast.Stop()
		o.Multicast = nil
	}
	if o.Admin != nil {
		_ = o.Admin.Stop()
		o.Admin = nil
	}
	o.stopCoreWithTimeout()
}

// //
