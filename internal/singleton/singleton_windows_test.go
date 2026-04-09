//go:build windows

package singleton

import (
	"fmt"
	"testing"
	"time"
)

func uniqueName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`Local\copynote-test-%s-%d`, t.Name(), time.Now().UnixNano())
}

func TestAcquire_FirstCallIsAlone(t *testing.T) {
	name := uniqueName(t)
	release, already, err := Acquire(name)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	if already {
		t.Errorf("first Acquire should report already=false")
	}
}

func TestAcquire_SecondCallSeesPrior(t *testing.T) {
	name := uniqueName(t)

	release1, already1, err := Acquire(name)
	if err != nil {
		t.Fatalf("Acquire #1: %v", err)
	}
	defer release1()
	if already1 {
		t.Errorf("first Acquire should report already=false")
	}

	release2, already2, err := Acquire(name)
	if err != nil {
		t.Fatalf("Acquire #2: %v", err)
	}
	defer release2()
	if !already2 {
		t.Errorf("second Acquire should report already=true while first still holds")
	}
}

func TestAcquire_FreshAfterRelease(t *testing.T) {
	name := uniqueName(t)

	release1, _, err := Acquire(name)
	if err != nil {
		t.Fatal(err)
	}
	release1() // release immediately

	release2, already, err := Acquire(name)
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	defer release2()
	if already {
		t.Errorf("after first handle was released, second Acquire should be solo (already=false)")
	}
}
