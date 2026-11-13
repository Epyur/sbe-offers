package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Тексты системы пишутся ПАРАЛЛЕЛЬНО (2026-11-14).
//
// Живая жалоба: «при выборе продукта бот уходит долго думать». Замер на боевом
// сервере: `description` 20.8/21.6 с, `advantages` 20.6/24.3 с —
// последовательно это ~45 секунд на КАЖДЫЙ выбранный продукт, и человек решает,
// что бот завис, и жмёт кнопку повторно.
//
// Проверяется сама раскладка блоков (writeBlocksInParallel): она не ходит в
// БД, поэтому Postgres для теста не нужен.

// blockingWriter отвечает по прошествии delay и запоминает, сколько вызовов
// шло в тот момент.
type blockingWriter struct {
	delay   time.Duration
	calls   int32
	inUse   int32
	overlap int32
}

func (w *blockingWriter) write(ctx context.Context, block string) (string, bool) {
	atomic.AddInt32(&w.calls, 1)
	now := atomic.AddInt32(&w.inUse, 1)
	for {
		old := atomic.LoadInt32(&w.overlap)
		if now <= old || atomic.CompareAndSwapInt32(&w.overlap, old, now) {
			break
		}
	}
	defer atomic.AddInt32(&w.inUse, -1)
	select {
	case <-time.After(w.delay):
	case <-ctx.Done():
		return "", false
	}
	return "текст " + block, true
}

func TestWriteBlocksInParallelRunsTogether(t *testing.T) {
	// 60 мс на блок: последовательно было бы 120 мс. Порог — меньше суммы и
	// заметно больше одного вызова, чтобы не зависеть от точности таймера.
	w := &blockingWriter{delay: 60 * time.Millisecond}
	started := time.Now()
	texts, missing := writeBlocksInParallel(context.Background(), systemTextBlocks, w.write)
	elapsed := time.Since(started)

	if len(missing) != 0 {
		t.Fatalf("блоки не написались: %v", missing)
	}
	for _, block := range systemTextBlocks {
		if texts[block] == "" {
			t.Fatalf("блок %q пуст: в документе не будет текста", block)
		}
	}
	if got := atomic.LoadInt32(&w.calls); got != int32(len(systemTextBlocks)) {
		t.Fatalf("модель вызвана %d раз, ожидалось %d", got, len(systemTextBlocks))
	}
	if atomic.LoadInt32(&w.overlap) < 2 {
		t.Fatal("вызовы шли по одному: блоки пишутся последовательно")
	}
	if elapsed >= 2*w.delay {
		t.Fatalf("запись длилась %v при %v на блок — блоки шли последовательно", elapsed, w.delay)
	}
}

// Авария одного блока не отменяет второй: выбор системы не должен проваливаться
// из-за модели, а недостающий блок честно попадает в `missing` в порядке блоков.
func TestWriteBlocksInParallelSurvivesFailure(t *testing.T) {
	var mu sync.Mutex
	failedOnce := false
	write := func(_ context.Context, block string) (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		if !failedOnce {
			failedOnce = true
			return "", false
		}
		return "текст " + block, true
	}
	texts, missing := writeBlocksInParallel(context.Background(), systemTextBlocks, write)
	if len(missing) != 1 {
		t.Fatalf("ожидали один недостающий блок, получили %v", missing)
	}
	if len(texts) != len(systemTextBlocks)-1 {
		t.Fatalf("написалось %d блоков из %d", len(texts), len(systemTextBlocks))
	}
	// Недостающий блок — из известных, а не выдуманный: по нему печатается
	// «какой блок не написался».
	if missing[0] != systemTextBlocks[0] && missing[0] != systemTextBlocks[1] {
		t.Fatalf("в missing попал неизвестный блок %q", missing[0])
	}
	if _, ok := texts[missing[0]]; ok {
		t.Fatalf("блок %q и написан, и недостаёт", missing[0])
	}
}

// Пустой список и отменённый запрос — не паника: блоки просто не пишутся.
func TestWriteBlocksInParallelHandlesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	write := func(ctx context.Context, block string) (string, bool) {
		select {
		case <-ctx.Done():
			return "", false
		default:
			return "текст " + block, true
		}
	}
	texts, missing := writeBlocksInParallel(ctx, systemTextBlocks, write)
	if len(texts) != 0 || len(missing) != len(systemTextBlocks) {
		t.Fatalf("отменённый запрос: написан %d, недостаёт %d", len(texts), len(missing))
	}
}
