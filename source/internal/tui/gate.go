// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/gate"
)

// gateDecision is how the startup notice resolved for this launch.
type gateDecision struct {
	proceed  bool   // false = the user quit at the gate
	readOnly bool   // open the session read-only
	banner   string // shown as a banner line in the main UI; empty = none
}

// runGate evaluates the startup posture and returns what the main UI should do.
// Full draws nothing; Nag draws nothing but returns a banner; Blocking shows a
// start screen the user either waits out (full read/write) or dismisses into
// read-only. screen is nil for the real terminal (tests pass a simulation one).
func runGate(screen tcell.Screen) gateDecision {
	return decideGate(gate.Evaluate(), screen, time.Now)
}

func decideGate(st gate.Status, screen tcell.Screen, now func() time.Time) gateDecision {
	switch st.Posture {
	case gate.Nag:
		return gateDecision{proceed: true, banner: st.Message}
	case gate.Blocking:
		return runBlockingGate(st, screen, now)
	default:
		return gateDecision{proceed: true}
	}
}

// runBlockingGate holds the user on a start screen. Read/write is withheld until
// WaitSeconds of real time has passed (Enter reports the remaining wait until
// then); 'd' dismisses straight into read-only; 'q'/Esc quits. There is no
// background ticker on purpose - every redraw happens on a key event, so the
// screen carries no goroutine that could outlive it.
func runBlockingGate(st gate.Status, screen tcell.Screen, now func() time.Time) gateDecision {
	app := tview.NewApplication()
	if screen != nil {
		app.SetScreen(screen)
	}
	unlockAt := now().Add(time.Duration(st.WaitSeconds) * time.Second)
	dec := gateDecision{} // default (quit) unless a key sets otherwise

	body := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	body.SetBorder(true).SetTitle(" notice ")

	draw := func() {
		remaining := int(unlockAt.Sub(now()).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		var action string
		if remaining > 0 {
			action = fmt.Sprintf("wait %ds, then [Enter] for read/write\n\n[d] continue read-only    [q] quit", remaining)
		} else {
			action = "[Enter] continue with read/write\n\n[d] continue read-only    [q] quit"
		}
		body.SetText("\n" + st.Message + "\n\n" + action)
	}
	draw()

	row := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(body, 62, 0, true).
		AddItem(nil, 0, 1, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(row, 9, 0, true).
		AddItem(nil, 0, 1, false)

	app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Rune() == 'q' || ev.Key() == tcell.KeyEscape:
			dec = gateDecision{}
			app.Stop()
		case ev.Rune() == 'd':
			dec = gateDecision{proceed: true, readOnly: true, banner: "read-only - " + st.Message}
			app.Stop()
		case ev.Key() == tcell.KeyEnter:
			if !now().Before(unlockAt) {
				dec = gateDecision{proceed: true, banner: st.Message}
				app.Stop()
				return nil
			}
			draw() // still waiting: refresh the remaining-seconds line
		}
		return nil
	})

	_ = app.SetRoot(root, true).Run()
	return dec
}
