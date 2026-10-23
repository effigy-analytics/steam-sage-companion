package app

import (
	"context"
	"errors"
	"io"
)

func RequestOpen(string) error { return errors.New("Windows activation is not yet verified") }
func RequestStop(string) error { return errors.New("Windows activation is not yet verified") }
func ListenActivation(context.Context, string, func() error, ...func()) (io.Closer, error) {
	return nil, nil
}
