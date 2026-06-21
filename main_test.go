package main

import (
	"reflect"
	"testing"
)

func TestParseScoresheetFixtureWeeks_Default(t *testing.T) {
	got, err := parseScoresheetFixtureWeeks("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestParseScoresheetFixtureWeeks_Range(t *testing.T) {
	got, err := parseScoresheetFixtureWeeks("3", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestParseScoresheetFixtureWeeks_All(t *testing.T) {
	got, err := parseScoresheetFixtureWeeks("all", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestParseScoresheetFixtureWeeks_Single(t *testing.T) {
	got, err := parseScoresheetFixtureWeeks("", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestParseScoresheetFixtureWeeks_Invalid(t *testing.T) {
	cases := []struct {
		name     string
		weeksArg string
		weekArg  string
	}{
		{name: "both", weeksArg: "3", weekArg: "2"},
		{name: "week zero", weekArg: "0"},
		{name: "weeks zero", weeksArg: "0"},
		{name: "weeks bad", weeksArg: "bad"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseScoresheetFixtureWeeks(tc.weeksArg, tc.weekArg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
