package guide

import (
	"os"
	"time"
)

type result struct {
	data *Snapshot
	err  error
}

// Service is owned by the game thread. Workers only send immutable snapshots.
type Service struct {
	Snapshot   *Snapshot
	Error      string
	Loading    bool
	result     chan result
	next       time.Time
	root, mode string
}

func (s *Service) Refresh(root, mode string) {
	if s.Loading {
		return
	}
	s.Loading = true
	s.root, s.mode = root, mode
	s.result = make(chan result, 1)
	ch := s.result
	go func() { d, e := Load(root, mode); ch <- result{d, e} }()
}
func (s *Service) Poll(root, mode string) bool {
	changed := false
	if s.Loading {
		select {
		case r := <-s.result:
			s.Loading = false
			s.Snapshot = r.data
			s.Error = ""
			if r.err != nil {
				s.Error = r.err.Error()
			}
			s.next = time.Now().Add(10 * time.Second)
			changed = true
		default:
		}
	}
	if !s.Loading && (s.next.IsZero() || s.root != root || s.mode != mode) {
		if s.root != root || s.mode != mode {
			s.Snapshot = nil
		}
		s.Refresh(root, mode)
		return true
	}
	if !s.Loading && time.Now().After(s.next) {
		s.next = time.Now().Add(10 * time.Second)
		old := s.Snapshot
		s.Loading = true
		ch := make(chan result, 1)
		s.result = ch
		go func() {
			if old != nil && !old.Changed() {
				ch <- result{old, nil}
				return
			}
			d, e := Load(root, mode)
			ch <- result{d, e}
		}()
	}
	return changed
}
func (s *Snapshot) Changed() bool {
	for path, old := range s.Files {
		st, err := os.Stat(path)
		if err != nil {
			if old != (Stamp{}) {
				return true
			}
			continue
		}
		if old != (Stamp{st.Size(), st.ModTime().UnixNano()}) {
			return true
		}
	}
	return false
}
