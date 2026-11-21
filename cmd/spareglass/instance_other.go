//go:build !windows

package main

func claimInstance() (release func(), err error) { return func() {}, nil }
