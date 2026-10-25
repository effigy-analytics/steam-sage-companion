package app

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"io"
)

func activationName(string) string { return "" }
func activationNameFor(string, string) (string, error) {
	return "", errors.New("Windows pipe identity is not yet verified")
}

var peerSID = func(windows.Handle) (string, error) {
	return "", errors.New("Windows peer identity is not yet verified")
}

func RequestStopAndWait(string, string) error {
	return errors.New("Windows stop and wait is not yet verified")
}
func RequestReadyFor(string, string) error {
	return errors.New("Windows readiness is not yet verified")
}
func RequestOpenFor(string, string) error {
	return errors.New("Windows activation is not yet verified")
}

func RequestReady(string) error { return errors.New("Windows readiness is not yet verified") }
func RequestOpen(string) error  { return errors.New("Windows activation is not yet verified") }
func RequestStop(string) error  { return errors.New("Windows activation is not yet verified") }
func ListenActivation(context.Context, string, func() error, ...func()) (io.Closer, error) {
	return nil, nil
}
