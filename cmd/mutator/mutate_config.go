package main

import (
	"github.com/goccy/go-yaml"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"strings"
)

type JsonPatch struct {
	Op    string      `json:"op" yaml:"op"`
	Path  string      `json:"path" yaml:"path"`
	Value interface{} `json:"value" yaml:"value"`
}

type PatchBase struct {
	Op    string `json:"op" yaml:"op"`
	Index string `json:"index" yaml:"index"`
}

type ContainerPatch struct {
	corev1.Container `json:",inline" yaml:",inline"`
	PatchBase        `json:",inline" yaml:",inline"`
}

type VolumePatch struct {
	corev1.Volume `json:",inline" yaml:",inline"`
	PatchBase     `json:",inline" yaml:",inline"`
}

type MutateConfig struct {
	Name           string           `json:"name" yaml:"name"`
	Patches        []JsonPatch      `json:"patch" yaml:"patch"`
	Containers     []ContainerPatch `json:"containers" yaml:"containers"`
	InitContainers []ContainerPatch `json:"initContainers" yaml:"initContainers"`
	Volumes        []VolumePatch    `json:"volumes" yaml:"volumes"`
}

func getDefaultedPatch(b PatchBase, p string, v interface{}) JsonPatch {
	path := p
	index := "-"
	if b.Index != "" {
		index = b.Index
	}
	path = path + index

	op := "add"
	if b.Op != "" {
		op = b.Op
	}

	var val interface{}
	val = v
	if op == "remove" {
		val = nil
	}

	return JsonPatch{Op: op, Path: path, Value: val}
}

func (c MutateConfig) GetPatch() []JsonPatch {
	patches := []JsonPatch{}

	for _, v := range c.Containers {
		patches = append(patches, getDefaultedPatch(v.PatchBase, "/spec/containers/", v.Container))
	}

	for _, v := range c.InitContainers {
		patches = append(patches, getDefaultedPatch(v.PatchBase, "/spec/initContainers/", v.Container))
	}

	for _, v := range c.Volumes {
		patches = append(patches, getDefaultedPatch(v.PatchBase, "/spec/volumes/", v.Volume))
	}

	patches = append(patches, c.Patches...)

	return patches
}

type MuteateConfigList []MutateConfig

func NewMutateConfigListFromKubernetes(client kubernetes.Interface, configCondition string) (MuteateConfigList, error) {
	// TODO: support more flexible targetCondition
	//       ex.) labelSelect, multiple, [{"label": ""}], ["a", "b"]

	configMap, err := client.CoreV1().ConfigMaps(corev1.NamespaceDefault).Get(configCondition, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	configs := []MutateConfig{}
	for key, value := range configMap.Data {
		mc := MutateConfig{Name: key}
		if err := yaml.Unmarshal([]byte(value), &mc); err != nil {
			continue
		}

		configs = append(configs, mc)
	}

	return configs, nil

}

func (c MuteateConfigList) GetPatch() []JsonPatch {
	patches := []JsonPatch{}
	for _, val := range c {
		patches = append(patches, val.GetPatch()...)
	}

	return patches
}

func hasPatchByPathPrefix(patches []JsonPatch, pathPrefix string) bool {
	for _, p := range patches {
		if strings.HasPrefix(p.Path, pathPrefix) {
			return true
		}
	}
	return false
}
