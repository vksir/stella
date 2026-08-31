package toolx

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
)

// 按规范路径串行化同一文件的写操作。
// agent 会并发执行同一轮的多个工具调用，并发编辑同一文件时队列保证顺序。

type mutationQueue struct {
	prev <-chan struct{}
}

func newMutationQueue() *mutationQueue {
	ch := make(chan struct{})
	close(ch) // 初始队列无前序操作，直接放行
	return &mutationQueue{prev: ch}
}

var (
	mutationMu     sync.Mutex
	mutationQueues = make(map[string]*mutationQueue)
)

// mutationKey 计算队列键：优先使用解析符号链接后的规范路径。
func mutationKey(absPath string) (string, error) {
	canonical, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return canonical, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return absPath, nil
	}
	return "", err
}

// withMutationQueue 等待同一路径的前序写操作完成后执行 fn。
func withMutationQueue(absPath string, fn func() error) error {
	key, err := mutationKey(absPath)
	if err != nil {
		return err
	}

	mutationMu.Lock()
	q := mutationQueues[key]
	if q == nil {
		q = newMutationQueue()
		mutationQueues[key] = q
	}
	prev := q.prev
	done := make(chan struct{})
	q.prev = done
	mutationMu.Unlock()

	// 等待前序操作完成，等待期间不响应取消。
	<-prev
	defer func() {
		close(done)
		mutationMu.Lock()
		if mutationQueues[key] == q {
			delete(mutationQueues, key)
		}
		mutationMu.Unlock()
	}()
	return fn()
}
