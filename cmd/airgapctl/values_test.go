package main

import (
	"reflect"
	"testing"
)

func TestExtractImageRefs_NoFalsePositives(t *testing.T) {
	input := `
some_repository: foo
myregistry: bar
imagePullPolicy: Always
`
	got := extractImageRefs(input)
	if len(got) != 0 {
		t.Errorf("expected no refs, got %v", got)
	}
}

func TestExtractImageRefs_RepositoryKey(t *testing.T) {
	input := `
repository: nginx
  repository: nginx
    repository: nginx
`
	got := extractImageRefs(input)
	want := []string{"nginx", "nginx", "nginx"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_ImageKey(t *testing.T) {
	input := `image: "docker.io/nginx:latest"`
	got := extractImageRefs(input)
	want := []string{"docker.io/nginx:latest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
