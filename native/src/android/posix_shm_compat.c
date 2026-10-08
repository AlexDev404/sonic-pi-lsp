// SPDX-License-Identifier: AGPL-3.0-or-later
// See posix_shm_compat.h.
#include "posix_shm_compat.h"
#include <errno.h>

int shm_open(const char* name, int oflag, mode_t mode) {
    (void)name; (void)oflag; (void)mode;
    errno = ENOSYS;
    return -1;
}

int shm_unlink(const char* name) {
    (void)name;
    errno = ENOSYS;
    return -1;
}
