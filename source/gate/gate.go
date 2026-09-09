// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package gate is the startup-notice seam between the open-source core and an
// external build that layers on a subscription. At startup a front-end asks the
// registered Provider for a Status and renders it: a Nag posture draws a banner
// line and the app runs normally; a Blocking posture holds the user on a start
// screen that either clears after a wait (full read/write) or is dismissed into
// read-only. The open-source build registers no provider, so Evaluate always
// returns Full and every front-end runs unimpeded - the core carries no notion
// of what any posture means, only how to draw the three of them.
//
// The seam is deliberately narrow and primitive-typed (ints and strings, no core
// types): the posture is computed entirely in the external build, behind this
// interface, so the open-source binary contains none of that logic. All this
// package knows is a posture number, a message, and a wait.
package gate

// Posture is how a front-end should treat the current session. The zero value is
// Full, so an unregistered provider (the open-source build) is always Full.
const (
	Full     = iota // run normally, no notice
	Nag             // run normally, but show Message as a banner
	Blocking        // hold on a start screen: wait out WaitSeconds -> read/write, or dismiss -> read-only
)

// Status is what a Provider reports for this run. Message is shown for Nag and
// Blocking; WaitSeconds is the countdown a Blocking start screen must run before
// it offers full read/write.
type Status struct {
	Posture     int
	Message     string
	WaitSeconds int
}

// Provider computes the session posture for one run. Evaluate calls it once at
// startup, so an implementation may treat each call as a distinct "open" (e.g.
// to advance an escalation counter). The external build registers one; the
// open-source build never does.
type Provider interface {
	Status() Status
}

var provider Provider

// Register installs the external provider. That build's main calls it once at
// startup, before dispatching; the open-source build never does.
func Register(p Provider) { provider = p }

// Available reports whether a provider is registered - i.e. whether this build
// can ever show anything but Full.
func Available() bool { return provider != nil }

// Evaluate returns the posture for this run, or Full when no provider is
// registered. Call it once per launch: a provider may count the call.
func Evaluate() Status {
	if provider == nil {
		return Status{Posture: Full}
	}
	return provider.Status()
}
