package launch

type Destination uint8

const (
	Sage Destination = iota
	Settings
	None
)

type Options struct {
	Executable, ConfigDir, ExtraOrigin string
	Destination                        Destination
}
