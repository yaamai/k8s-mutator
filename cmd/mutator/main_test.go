package main

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func TestMutateConfigGetPatch(t *testing.T) {
	tests := []struct {
		desc   string
		in     MutateConfig
		expect []JsonPatch
		err    error
	}{
		{desc: "empty",
			in: MutateConfig{}, expect: []JsonPatch{}, err: nil},
		{desc: "empty patches",
			in: MutateConfig{Patches: []JsonPatch{}}, expect: []JsonPatch{}, err: nil},
		{desc: "one patches",
			in:     MutateConfig{Patches: []JsonPatch{{Op: "add", Path: "/foobar", Value: "baz"}}},
			expect: []JsonPatch{JsonPatch{Op: "add", Path: "/foobar", Value: "baz"}},
			err:    nil},
		{desc: "one container",
			in:     MutateConfig{Containers: []ContainerPatch{ContainerPatch{Container: corev1.Container{Name: "foobar"}, PatchBase: PatchBase{Op: "add", Index: "-"}}}},
			expect: []JsonPatch{{Op: "add", Path: "/spec/containers/-", Value: corev1.Container{Name: "foobar"}}},
			err:    nil},
		{desc: "one container inserted to 0",
			in:     MutateConfig{Containers: []ContainerPatch{ContainerPatch{Container: corev1.Container{Name: "foobar"}, PatchBase: PatchBase{Op: "add", Index: "0"}}}},
			expect: []JsonPatch{{Op: "add", Path: "/spec/containers/0", Value: corev1.Container{Name: "foobar"}}},
			err:    nil},
		{desc: "one container removed from 0",
			in:     MutateConfig{Containers: []ContainerPatch{ContainerPatch{Container: corev1.Container{Name: "foobar"}, PatchBase: PatchBase{Op: "remove", Index: "0"}}}},
			expect: []JsonPatch{{Op: "remove", Path: "/spec/containers/0", Value: nil}},
			err:    nil},
		{desc: "one container replace 0",
			in:     MutateConfig{Containers: []ContainerPatch{ContainerPatch{Container: corev1.Container{Name: "foobar"}, PatchBase: PatchBase{Op: "replace", Index: "0"}}}},
			expect: []JsonPatch{{Op: "replace", Path: "/spec/containers/0", Value: corev1.Container{Name: "foobar"}}},
			err:    nil},
	}
	for _, tt := range tests {
		actual := tt.in.GetPatch()
		// assert.Equal(t, tt.err, err)
		assert.Equal(t, tt.expect, actual)
	}
}
