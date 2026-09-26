//go:build race

package pdbcompat

// raceEnabled reports whether the test binary runs with the race
// detector, which changes the size of allocations.
const raceEnabled = true
