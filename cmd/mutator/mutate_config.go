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

type JsonPatchSet []JsonPatch

func (s JsonPatchSet) HasPrefix(pathPrefix string) bool {
	for _, p := range s {
		if strings.HasPrefix(p.Path, pathPrefix) {
			return true
		}
	}
	return false
}

func (s *JsonPatchSet) Prepend(p JsonPatch) {
	*s = append(*s, JsonPatch{})
	copy((*s)[1:], *s)
	(*s)[0] = p
}

func (s *JsonPatchSet) AddNonExistentPathPatch(pod *corev1.Pod) {
	if pod.Spec.InitContainers == nil && (s.HasPrefix("/spec/initContainers/")) {
		s.Prepend(JsonPatch{Op: "add", Path: "/spec/initContainers", Value: []interface{}{}})
	}
	if pod.Spec.Containers == nil && s.HasPrefix("/spec/containers/") {
		s.Prepend(JsonPatch{Op: "add", Path: "/spec/containers", Value: []interface{}{}})
	}
	if pod.Spec.Volumes == nil && s.HasPrefix("/spec/volumes/") {
		s.Prepend(JsonPatch{Op: "add", Path: "/spec/volumes", Value: []interface{}{}})
	}
}

type PatchBase struct {
	Op    string `json:"op" yaml:"op"`
	Index string `json:"index" yaml:"index"`
}

func (p PatchBase) Expand(pathBase string, v interface{}) JsonPatch {
	path := pathBase
	index := "-"
	if p.Index != "" {
		index = p.Index
	}
	path = path + index

	op := "add"
	if p.Op != "" {
		op = p.Op
	}

	var val interface{}
	val = v
	if op == "remove" {
		val = nil
	}

	return JsonPatch{Op: op, Path: path, Value: val}
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

func (c MutateConfig) GetJsonPatchSet() JsonPatchSet {
	patches := JsonPatchSet{}

	for _, v := range c.Containers {
		patches = append(patches, v.PatchBase.Expand("/spec/containers/", v.Container))
	}

	for _, v := range c.InitContainers {
		patches = append(patches, v.PatchBase.Expand("/spec/initContainers/", v.Container))
	}

	for _, v := range c.Volumes {
		patches = append(patches, v.PatchBase.Expand("/spec/volumes/", v.Volume))
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

func (c MuteateConfigList) GetJsonPatchSet() JsonPatchSet {
	patches := JsonPatchSet{}
	for _, val := range c {
		patches = append(patches, val.GetJsonPatchSet()...)
	}

	return patches
}
