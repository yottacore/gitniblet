// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Background git sync for the long-running front-ends, paced by the
// git_sync_frequency tunable. The post-sync apply is a full replay - the
// safe handler until incremental apply exists (a pulled entry may sort before
// entries already applied).
package schema

import (
	"context"
	"fmt"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// StartAutoSync begins the sync loop and returns its stop function. It is a
// no-op (returning a working stop) when freq <= 0 or the log dir is not a
// git work tree. onWarn receives sync errors and replay warnings - the loop
// itself never gives up.
func (c *Client) StartAutoSync(freq int, onWarn func(string)) func() {
	if freq <= 0 || !txlog.InRepo(c.log.Dir()) {
		return func() {}
	}
	if onWarn == nil {
		onWarn = func(string) {}
	}
	syncer := txlog.NewSyncer(c.log, time.Duration(freq)*time.Second)
	syncer.OnChange = func() error {
		entries, readWarns, err := c.log.ReadAll()
		if err != nil {
			return err
		}
		builtins, err := Builtins()
		if err != nil {
			return err
		}
		ApplyAliases(entries, c.Schema, builtins)
		// Through the API, not txlog.Apply: a pull rebuilds the whole view, and
		// doing that on the sync goroutine could land between a write's log
		// append and its own apply - the interleaving the write lock exists to
		// prevent.
		applyWarns, err := c.API.Replay(entries)
		for _, warn := range append(readWarns, applyWarns...) {
			onWarn(warn)
		}
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go syncer.Run(ctx, func(err error) { onWarn(fmt.Sprintf("sync: %v", err)) })
	return cancel
}
