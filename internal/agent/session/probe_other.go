//go:build !darwin && !windows

package session

import "context"

const supported = false

func probe(context.Context) (Sample, error) { return Sample{}, ErrUnsupported }
