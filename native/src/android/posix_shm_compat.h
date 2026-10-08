// SPDX-License-Identifier: AGPL-3.0-or-later
// Android's libc has no POSIX shared memory (shm_open, shm_unlink). clockwork uses them to share its arena with a
// client in another process; in the app the engine and its one client share
// a process (clockwork_client_open_memory), so those paths never run. This
// header is forced into clockwork's sources on Android (native/CMakeLists.txt)
// and posix_shm_compat.c answers them: they refuse.
#pragma once
#if defined(__ANDROID__)
#include <sys/types.h>
#ifdef __cplusplus
extern "C" {
#endif
int shm_open(const char* name, int oflag, mode_t mode);
int shm_unlink(const char* name);
#ifdef __cplusplus
}
#endif
#endif
