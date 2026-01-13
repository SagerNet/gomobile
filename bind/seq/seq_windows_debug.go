//go:build windows

package seq

/*
#cgo windows CFLAGS: -D__GOBIND_WINDOWS__
#include <windows.h>
#include <stdio.h>

static void seq_debug_output(const char* message) {
        OutputDebugStringA(message);
}

static LONG WINAPI seq_crash_handler(EXCEPTION_POINTERS* exceptionInfo) {
        if (exceptionInfo == NULL || exceptionInfo->ExceptionRecord == NULL) {
                OutputDebugStringA("gomobile: unhandled exception\n");
                return EXCEPTION_CONTINUE_SEARCH;
        }
        DWORD exceptionCode = exceptionInfo->ExceptionRecord->ExceptionCode;
        void* exceptionAddress = exceptionInfo->ExceptionRecord->ExceptionAddress;
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
*/
import "C"

import (
	"unsafe"
	_ "unsafe" // Required for go:linkname
)

//go:linkname runtimeOverrideWrite runtime.overrideWrite
var runtimeOverrideWrite func(fd uintptr, p unsafe.Pointer, n int32) int32

// debugWrite intercepts stderr output and forwards it to OutputDebugString.
//
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
}
