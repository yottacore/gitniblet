// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package soak holds the soak test, which runs open, write, sync and replay
// over and over, and fails when heap in use or the goroutine count keeps
// climbing. It runs only when GITNIBLET_SOAK gives it a time, as cicd.bash's
// Soak test step does. Only its tests have code.
package soak
