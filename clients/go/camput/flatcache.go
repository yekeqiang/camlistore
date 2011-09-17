/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"gob"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"camli/client"
	"camli/osutil"
)

type fileInfoPutRes struct {
	Fi os.FileInfo
	Pr client.PutResult
}

// FlatCache is an ugly hack, until leveldb-go is ready
// (http://code.google.com/p/leveldb-go/)
type FlatCache struct {
	mu sync.Mutex
	filename string
	m        map[string]fileInfoPutRes
	dirty    map[string]fileInfoPutRes
}

func NewFlatCache() *FlatCache {
	filename := filepath.Join(osutil.CacheDir(), "camput.cache")
	fc := &FlatCache{
		filename: filename,
		m:        make(map[string]fileInfoPutRes),
		dirty: make(map[string]fileInfoPutRes),
	}

	if f, err := os.Open(filename); err == nil {
		defer f.Close()
		d := gob.NewDecoder(f)
		for {
			var key string
			var val fileInfoPutRes
			if d.Decode(&key) != nil || d.Decode(&val) != nil {
				break
			}
			val.Pr.Skipped = true
			fc.m[key] = val
		}
	}
	return fc
}

var _ UploadCache = (*FlatCache)(nil)

var ErrCacheMiss = os.NewError("not in cache")

// filename may be relative.
// returns ErrCacheMiss on miss
func cacheKey(pwd, filename string) string {
	return pwd + "\x00" + filename
}

func (c *FlatCache) CachedPutResult(pwd, filename string, fi *os.FileInfo) (*client.PutResult, os.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	val, ok := c.m[cacheKey(pwd, filename)]
	if !ok {
		return nil, ErrCacheMiss
	}
	if !reflect.DeepEqual(val.Fi, fi) {
		return nil, ErrCacheMiss
	}
	pr := val.Pr
	return &pr, nil
}

func (c *FlatCache) AddCachedPutResult(pwd, filename string, fi *os.FileInfo, pr *client.PutResult) {
	c.mu.Lock()
        defer c.mu.Unlock()
	key := cacheKey(pwd, filename)

	vprintf("Adding to stat cache %q: %v", filename, pr)

	c.dirty[key] = fileInfoPutRes{*fi, *pr}
	c.m[key] = fileInfoPutRes{*fi, *pr}
}

func (c *FlatCache) Save() {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.OpenFile(c.filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("FlatCache OpenFile: %v", err)
	}
	defer f.Close()
	e := gob.NewEncoder(f)
	write := func(v interface{}) {
		if err := e.Encode(v); err != nil {
			panic("Encode: " + err.String())
		}
	}
	for k, v := range c.dirty {
		write(k)
		write(v)
	}
	c.dirty = make(map[string]fileInfoPutRes)
}