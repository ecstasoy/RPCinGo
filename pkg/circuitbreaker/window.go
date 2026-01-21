// Kunhua Huang 2026

package circuitbreaker

import (
	"sync"
	"time"
)

type Bucket struct {
	success int64
	failure int64
	timeout int64
	total   int64
}

type SlidingWindow struct {
	buckets    []Bucket
	size       int
	bucketTime time.Duration

	currentIndex int
	lastUpdate   time.Time
	mu           sync.RWMutex
}

func NewSlidingWindow(size int, bucketTime time.Duration) *SlidingWindow {
	return &SlidingWindow{
		buckets:    make([]Bucket, size),
		size:       size,
		bucketTime: bucketTime,
		lastUpdate: time.Now(),
	}
}

func (s *SlidingWindow) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateBuckets()
	s.buckets[s.currentIndex].success++
	s.buckets[s.currentIndex].total++
}

func (s *SlidingWindow) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateBuckets()
	s.buckets[s.currentIndex].failure++
	s.buckets[s.currentIndex].total++
}

func (s *SlidingWindow) RecordTimeout() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateBuckets()
	s.buckets[s.currentIndex].timeout++
	s.buckets[s.currentIndex].total++
}

func (s *SlidingWindow) updateBuckets() {
	now := time.Now()
	elapsed := now.Sub(s.lastUpdate)

	if elapsed < s.bucketTime {
		return
	}

	bucketsToAdvance := int(elapsed / s.bucketTime)
	if bucketsToAdvance >= s.size {
		for i := range s.buckets {
			s.buckets[i] = Bucket{}
		}
		s.currentIndex = 0
	} else {
		for i := 0; i < bucketsToAdvance; i++ {
			s.currentIndex = (s.currentIndex + 1) % s.size
			s.buckets[s.currentIndex] = Bucket{}
		}
	}

	s.lastUpdate = now
}

func (s *SlidingWindow) Stats() (success, failure, timeout, total int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, bucket := range s.buckets {
		success += bucket.success
		failure += bucket.failure
		timeout += bucket.timeout
		total += bucket.total
	}

	return
}

func (s *SlidingWindow) FailureRate() float64 {
	_, failure, timeout, total := s.Stats()

	if total == 0 {
		return 0
	}

	return float64(failure+timeout) / float64(total)
}

func (s *SlidingWindow) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.buckets {
		s.buckets[i] = Bucket{}
	}
}

func (s *SlidingWindow) Total() int64 {
	_, _, _, total := s.Stats()
	return total

}
