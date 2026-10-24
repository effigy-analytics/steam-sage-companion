package launch

import "errors"

type Options struct{ Executable, ConfigDir, ExtraOrigin string }

func Run(Options) error { return errors.New("Windows background launch is not yet supported") }
