// Parse auditd log files.

// Auditd writes multiple lines for the same event. We therefore need
// to group the lines and emit a single event row.

package syslog

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/elastic/go-libaudit"
	"github.com/elastic/go-libaudit/aucoalesce"
	"github.com/elastic/go-libaudit/auparse"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type AuditdPlugin struct{}

func (self AuditdPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_auditd",
		Doc:     "Parse log files generated by auditd.",
		ArgType: type_map.AddType(scope, &ScannerPluginArgs{}),
	}
}

func (self AuditdPlugin) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		reassembler, err := libaudit.NewReassembler(5, 2*time.Second,
			&streamHandler{scope, output_chan})
		if err != nil {
			scope.Log("parse_auditd: %v", err)
			return
		}
		defer reassembler.Close()

		// Start goroutine to periodically purge timed-out events.
		go func() {
			t := time.NewTicker(500 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return

				case <-t.C:
					if reassembler.Maintain() != nil {
						return
					}
				}
			}
		}()

		scanner := ScannerPlugin{}
		for row := range scanner.Call(ctx, scope, args) {
			line, pres := scope.Associative(row, "Line")
			if !pres {
				continue
			}

			auditMsg, err := auparse.ParseLogLine(line.(string))
			if err == nil {
				reassembler.PushMessage(auditMsg)
			}
		}
	}()

	return output_chan
}

type streamHandler struct {
	scope       *vfilter.Scope
	output_chan chan vfilter.Row
}

func (self *streamHandler) ReassemblyComplete(msgs []*auparse.AuditMessage) {
	self.outputMultipleMessages(msgs)
}

func (self *streamHandler) EventsLost(count int) {
	self.scope.Log("Detected the loss of %v sequences.", count)
}

func (self *streamHandler) outputMultipleMessages(msgs []*auparse.AuditMessage) {
	event, err := aucoalesce.CoalesceMessages(msgs)
	if err != nil {
		return
	}
	self.output_chan <- event
}

type WatchAuditdPlugin struct{}

func (self WatchAuditdPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch_auditd",
		Doc:     "Watch log files generated by auditd.",
		ArgType: type_map.AddType(scope, &ScannerPluginArgs{}),
	}
}

func (self WatchAuditdPlugin) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		reassembler, err := libaudit.NewReassembler(5, 2*time.Second,
			&streamHandler{scope, output_chan})
		if err != nil {
			scope.Log("watch_auditd: %v", err)
			return
		}
		defer reassembler.Close()

		// Start goroutine to periodically purge timed-out events.
		go func() {
			t := time.NewTicker(500 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return

				case <-t.C:
					if reassembler.Maintain() != nil {
						return
					}
				}
			}
		}()

		scanner := _WatchSyslogPlugin{}
		for row := range scanner.Call(ctx, scope, args) {
			line, pres := scope.Associative(row, "Line")
			if !pres {
				continue
			}

			auditMsg, err := auparse.ParseLogLine(line.(string))
			if err == nil {
				reassembler.PushMessage(auditMsg)
			}
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&AuditdPlugin{})
	vql_subsystem.RegisterPlugin(&WatchAuditdPlugin{})
}
