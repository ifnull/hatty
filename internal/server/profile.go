package server

import "github.com/muesli/termenv"

// The design assumes 256 colours, verified on the panel by spike S2. Forcing
// the profile keeps rendering identical regardless of what a client claims,
// which is what makes golden-file tests meaningful.
const termProfile = termenv.ANSI256
