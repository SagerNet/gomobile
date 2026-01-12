// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef __GO_SEQ_WINDOWS_HDR__
#define __GO_SEQ_WINDOWS_HDR__

#include <stdint.h>
#include <stdlib.h>

#if defined(_WIN32)
#define SEQ_EXPORT __declspec(dllexport)
#else
#define SEQ_EXPORT
#endif

typedef struct nstring {
	void *ptr;
	int len;
} nstring;

typedef struct nbyteslice {
	void *ptr;
	int len;
} nbyteslice;

typedef int64_t nint;

typedef void (*go_seq_ref_fn)(int32_t refnum);

// Initialize the Go<=>C# binding layer. Must be called before any other go_seq_* function.
SEQ_EXPORT void go_seq_init(void);
SEQ_EXPORT void go_seq_inc_ref(int32_t refnum);
SEQ_EXPORT void go_seq_dec_ref(int32_t refnum);
SEQ_EXPORT void go_seq_set_inc_ref(go_seq_ref_fn fn);
SEQ_EXPORT void go_seq_set_dec_ref(go_seq_ref_fn fn);

#endif // __GO_SEQ_WINDOWS_HDR__
