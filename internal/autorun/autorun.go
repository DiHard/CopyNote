// Package autorun manages the command that starts this installation when the
// user signs in: a Run registry value on Windows, a LaunchAgent on macOS.
package autorun

// AutostartFlag distinguishes sign-in from an explicit user launch.
const AutostartFlag = "--autostart"
