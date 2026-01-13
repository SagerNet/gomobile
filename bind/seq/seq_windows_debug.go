//go:build windows

package seq

/*
#cgo windows CFLAGS: -D__GOBIND_WINDOWS__
#include <windows.h>
#include <stdio.h>
#include <stdlib.h>

#ifndef DBG_PRINTEXCEPTION_C
#define DBG_PRINTEXCEPTION_C 0x40010006
#endif

#ifndef DBG_PRINTEXCEPTION_WIDE_C
#define DBG_PRINTEXCEPTION_WIDE_C 0x4001000A
#endif

static void seq_debug_output(const char* message) {
	OutputDebugStringA(message);
}

static int seq_debug_enabled(void) {
	char value[2];
	DWORD len = GetEnvironmentVariableA("GOMOBILE_DEBUG_OUTPUT", value, sizeof(value));
	return len > 0;
}


static LONG WINAPI seq_crash_handler(EXCEPTION_POINTERS* exceptionInfo) {
        if (exceptionInfo == NULL || exceptionInfo->ExceptionRecord == NULL) {
                OutputDebugStringA("gomobile: unhandled exception\n");
                return EXCEPTION_CONTINUE_SEARCH;
        }
        DWORD exceptionCode = exceptionInfo->ExceptionRecord->ExceptionCode;
        void* exceptionAddress = exceptionInfo->ExceptionRecord->ExceptionAddress;
        if (exceptionCode == DBG_PRINTEXCEPTION_C || exceptionCode == DBG_PRINTEXCEPTION_WIDE_C) {
                return EXCEPTION_CONTINUE_SEARCH;
        }
        static LONG seq_in_exception_handler = 0;
        if (InterlockedExchange(&seq_in_exception_handler, 1) != 0) {
                return EXCEPTION_CONTINUE_SEARCH;
        }
        char message[256];
        int length = snprintf(
                message,
                sizeof(message),
                "gomobile: unhandled exception 0x%08lX at %p\n",
                exceptionCode,
                exceptionAddress
        );
        if (length > 0) {
                OutputDebugStringA(message);
        } else {
                OutputDebugStringA("gomobile: unhandled exception\n");
        }
        InterlockedExchange(&seq_in_exception_handler, 0);
        return EXCEPTION_CONTINUE_SEARCH;
}

static void* seq_crash_handler_handle = NULL;

static void seq_register_crash_handler(void) {
	if (seq_crash_handler_handle != NULL) {
		return;
	}
        seq_crash_handler_handle = AddVectoredExceptionHandler(1, seq_crash_handler);
        if (seq_crash_handler_handle == NULL) {
                DWORD errorCode = GetLastError();
                char message[128];
	int length = snprintf(
		message,
		sizeof(message),
		"gomobile: AddVectoredExceptionHandler failed: %lu\n",
		errorCode
	);
                if (length > 0) {
                        OutputDebugStringA(message);
                } else {
                        OutputDebugStringA("gomobile: AddVectoredExceptionHandler failed\n");
                }
	}
}

#if defined(__GNUC__)
__attribute__((constructor))
#endif
static void seq_debug_constructor(void) {
	if (seq_debug_enabled()) {
		OutputDebugStringA("gomobile: cgo constructor\n");
	}
	seq_register_crash_handler();
}
*/
import "C"

import (
	"os"
	"unsafe"
	_ "unsafe" // Required for go:linkname
)

//go:linkname runtimeOverrideWrite runtime.overrideWrite
var runtimeOverrideWrite func(fd uintptr, p unsafe.Pointer, n int32) int32

//go:nosplit
func debugWrite(fd uintptr, p unsafe.Pointer, n int32) int32 {
	if fd != 2 {
		return n
	}

	const maxChunk = 256
	input := (*[1 << 30]byte)(p)[:n]
	for len(input) > 0 {
		chunkSize := len(input)
		if chunkSize > maxChunk-1 {
			chunkSize = maxChunk - 1
		}

		var buffer [maxChunk]byte
		copy(buffer[:], input[:chunkSize])
		buffer[chunkSize] = 0
		C.seq_debug_output((*C.char)(unsafe.Pointer(&buffer[0])))
		input = input[chunkSize:]
	}

	return n
}

func init() {
	C.seq_register_crash_handler()
	runtimeOverrideWrite = debugWrite
	if os.Getenv("GOMOBILE_DEBUG_OUTPUT") != "" {
		message := C.CString("gomobile: debug hooks installed\n")
		C.seq_debug_output(message)
		C.free(unsafe.Pointer(message))
	}
}
