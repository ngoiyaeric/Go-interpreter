module github.com/jjviana/codex

go 1.16

replace github.com/ActiveState/vt10x v1.3.2 => github.com/jjviana/vt10x v1.3.2

require (
	github.com/ActiveState/vt10x v1.3.2
	github.com/aws/aws-sdk-go v1.44.194 // indirect
	github.com/creack/pty v1.1.17
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gdamore/tcell/v2 v2.4.1-0.20210905002822-f057f0a857a1
	github.com/rivo/tview v0.0.0-20211202162923-2a6de950f73b
	github.com/rs/zerolog v1.26.0
	github.com/stretchr/testify v1.2.2 // indirect
	golang.org/x/sys v0.1.0
	golang.org/x/term v0.1.0
)
