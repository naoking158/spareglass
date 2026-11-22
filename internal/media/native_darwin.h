#ifndef SPAREGLASS_NATIVE_DARWIN_H
#define SPAREGLASS_NATIVE_DARWIN_H

#include <stdint.h>

typedef struct sgEncoder sgEncoder;
typedef struct sgCapture sgCapture;

// Encoded frames and errors are delivered to these Go functions with the
// handle given at creation.
extern void sgDeliverFrame(uintptr_t handle, void *data, int len, int64_t pts_ns, int keyframe);
extern void sgDeliverError(uintptr_t handle, char *msg);

sgEncoder *sgEncoderNew(int width, int height, int fps, int kbps, uintptr_t handle, char **err);
// sgEncoderEncodeTestFrame encodes a generated NV12 frame (for tests).
int sgEncoderEncodeTestFrame(sgEncoder *e, int index, int64_t pts_ns);
void sgEncoderFree(sgEncoder *e);

sgCapture *sgCaptureStart(uint32_t display_id, int width, int height, int fps, int kbps, uintptr_t handle, char **err);
void sgCaptureRequestKeyframe(sgCapture *c);
void sgCaptureStop(sgCapture *c);

// sgCursorLocation stores the pointer position in global display points.
void sgCursorLocation(double *x, double *y);
// sgDisplayBounds stores a display's rectangle in global display points
// (all zero for an unknown display). It can take tens of milliseconds, so
// callers cache it.
void sgDisplayBounds(uint32_t display_id, double *x, double *y, double *w, double *h);

#endif
