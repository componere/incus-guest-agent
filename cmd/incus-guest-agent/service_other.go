//go:build !linux

package main

import (
	"context"
	"errors"
)

// runService reports that the supervisor requires Linux.
func runService(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return errors.New("incus-guest-agent requires linux")
}
