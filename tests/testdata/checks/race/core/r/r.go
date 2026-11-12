// Package r counts.
package r

// Count is shared by everyone who calls Add.
var Count int

// Add adds one to Count, with no lock.
func Add() { Count++ }
