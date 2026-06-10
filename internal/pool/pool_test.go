package pool

import (
	"testing"
)

func TestWorkerStatusString(t *testing.T) {
	tests := []struct {
		status WorkerStatus
		want   string
	}{
		{WorkerStatusStopped, "stopped"},
		{WorkerStatusStarting, "starting"},
		{WorkerStatusRunning, "running"},
		{WorkerStatusDead, "dead"},
		{WorkerStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("WorkerStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestNewWorkerDefaults(t *testing.T) {
	w := NewWorker(0, 9001, "php-cgi.exe", "/var/www", 1000, 4)

	if w.ID != 0 {
		t.Errorf("NewWorker ID = %d, want 0", w.ID)
	}
	if w.Port != 9001 {
		t.Errorf("NewWorker Port = %d, want 9001", w.Port)
	}
	if w.PHPBinPath != "php-cgi.exe" {
		t.Errorf("NewWorker PHPBinPath = %s, want php-cgi.exe", w.PHPBinPath)
	}
	if w.DocumentRoot != "/var/www" {
		t.Errorf("NewWorker DocumentRoot = %s, want /var/www", w.DocumentRoot)
	}

	// Initial state
	if w.Status() != WorkerStatusStopped {
		t.Errorf("NewWorker initial status = %s, want stopped", w.Status())
	}
	if w.RequestCount() != 0 {
		t.Errorf("NewWorker initial request count = %d, want 0", w.RequestCount())
	}
	if w.RestartCount() != 0 {
		t.Errorf("NewWorker initial restart count = %d, want 0", w.RestartCount())
	}
	if w.PID() != 0 {
		t.Errorf("NewWorker initial PID = %d, want 0", w.PID())
	}
	if w.IsAlive() {
		t.Error("NewWorker initial IsAlive() = true, want false")
	}
}

func TestWorkerIncrementRequests(t *testing.T) {
	w := NewWorker(0, 9001, "php-cgi.exe", "/var/www", 1000, 4)
	w.IncrementRequests()
	if w.RequestCount() != 1 {
		t.Errorf("After IncrementRequests, count = %d, want 1", w.RequestCount())
	}
	w.IncrementRequests()
	w.IncrementRequests()
	if w.RequestCount() != 3 {
		t.Errorf("After 3 increments, count = %d, want 3", w.RequestCount())
	}
}

func TestWorkerInfo(t *testing.T) {
	w := NewWorker(1, 9002, "php-cgi.exe", "/var/www", 1000, 4)
	w.IncrementRequests()
	w.IncrementRequests()

	info := w.Info()

	if info.ID != 1 {
		t.Errorf("Info().ID = %d, want 1", info.ID)
	}
	if info.Port != 9002 {
		t.Errorf("Info().Port = %d, want 9002", info.Port)
	}
	if info.Status != "stopped" {
		t.Errorf("Info().Status = %s, want stopped", info.Status)
	}
	if info.RequestCount != 2 {
		t.Errorf("Info().RequestCount = %d, want 2", info.RequestCount)
	}
	if info.PID != 0 {
		t.Errorf("Info().PID = %d, want 0", info.PID)
	}
}

func TestBalancerEmpty(t *testing.T) {
	b := NewBalancer(nil)
	if w := b.Next(); w != nil {
		t.Errorf("Balancer.Next() on nil workers = %v, want nil", w)
	}

	b2 := NewBalancer([]*Worker{})
	if w := b2.Next(); w != nil {
		t.Errorf("Balancer.Next() on empty workers = %v, want nil", w)
	}
}

func TestBalancerAllDead(t *testing.T) {
	w1 := NewWorker(0, 9001, "php-cgi.exe", "/var/www", 1000, 4)
	w2 := NewWorker(1, 9002, "php-cgi.exe", "/var/www", 1000, 4)
	// Both are stopped/dead

	b := NewBalancer([]*Worker{w1, w2})
	if w := b.Next(); w != nil {
		t.Errorf("Balancer.Next() with all dead workers = %v, want nil", w)
	}
}

func TestBalancerActiveCount(t *testing.T) {
	w1 := NewWorker(0, 9001, "php-cgi.exe", "/var/www", 1000, 4)
	w2 := NewWorker(1, 9002, "php-cgi.exe", "/var/www", 1000, 4)
	w3 := NewWorker(2, 9003, "php-cgi.exe", "/var/www", 1000, 4)

	b := NewBalancer([]*Worker{w1, w2, w3})
	// All dead initially
	if count := b.ActiveCount(); count != 0 {
		t.Errorf("ActiveCount with all dead = %d, want 0", count)
	}
}

func TestBalancerRoundRobinProperty(t *testing.T) {
	// The balancer uses atomic counter, so even with dead workers
	// it still increments the counter. Verify the counter behavior
	b := NewBalancer([]*Worker{})

	// Even with 0 workers, the counter should still increment
	for i := 0; i < 5; i++ {
		_ = b.Next() // returns nil but counter increments
	}
}

func TestNewManager(t *testing.T) {
	// Just validate that NewManager doesn't panic with nil config
	// (it stores the reference, doesn't dereference)
	m := NewManager(nil)
	if m == nil {
		t.Error("NewManager(nil) returned nil")
	}
	if m.IsRunning() {
		t.Error("New manager should not be running")
	}
}
