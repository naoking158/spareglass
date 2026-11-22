//go:build darwin && cgo

#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <VideoToolbox/VideoToolbox.h>
#include <stdlib.h>
#include <string.h>

#include "native_darwin.h"

static char *copyCString(NSString *s) { return strdup(s.UTF8String ?: "unknown error"); }

static NSString *statusText(NSString *what, OSStatus st) {
	return [NSString stringWithFormat:@"%@: OSStatus %d", what, (int)st];
}

// ---- Encoder: VideoToolbox H.264, one output per input, Annex-B out ----

struct sgEncoder {
	VTCompressionSessionRef session;
	uintptr_t handle;
	int width, height;
};

static const uint8_t startCode[4] = {0, 0, 0, 1};

static void appendParameterSets(NSMutableData *out, CMFormatDescriptionRef fmt) {
	size_t count = 0;
	if (CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, NULL, NULL, &count, NULL) != noErr) return;
	for (size_t i = 0; i < count; i++) {
		const uint8_t *ps = NULL;
		size_t size = 0;
		if (CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, i, &ps, &size, NULL, NULL) == noErr) {
			[out appendBytes:startCode length:4];
			[out appendBytes:ps length:size];
		}
	}
}

static void encoderOutput(void *refCon, void *frameRefCon, OSStatus status, VTEncodeInfoFlags flags, CMSampleBufferRef sb) {
	sgEncoder *e = refCon;
	if (status != noErr) {
		char *msg = copyCString(statusText(@"VideoToolbox encode", status));
		sgDeliverError(e->handle, msg);
		free(msg);
		return;
	}
	if (sb == NULL || (flags & kVTEncodeInfo_FrameDropped)) return;

	BOOL key = YES;
	CFArrayRef atts = CMSampleBufferGetSampleAttachmentsArray(sb, false);
	if (atts && CFArrayGetCount(atts) > 0) {
		CFDictionaryRef a = CFArrayGetValueAtIndex(atts, 0);
		key = !CFDictionaryContainsKey(a, kCMSampleAttachmentKey_NotSync);
	}

	CMFormatDescriptionRef fmt = CMSampleBufferGetFormatDescription(sb);
	int lenSize = 4;
	CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, NULL, NULL, NULL, &lenSize);

	NSMutableData *out = [NSMutableData data];
	if (key) appendParameterSets(out, fmt);

	CMBlockBufferRef bb = CMSampleBufferGetDataBuffer(sb);
	size_t total = CMBlockBufferGetDataLength(bb), contiguous = 0;
	char *p = NULL;
	if (CMBlockBufferGetDataPointer(bb, 0, &contiguous, NULL, &p) != noErr) return;
	NSMutableData *copy = nil;
	if (contiguous < total) {
		copy = [NSMutableData dataWithLength:total];
		if (CMBlockBufferCopyDataBytes(bb, 0, total, copy.mutableBytes) != noErr) return;
		p = copy.mutableBytes;
	}
	size_t off = 0;
	while (off + lenSize <= total) {
		uint32_t n = 0;
		for (int i = 0; i < lenSize; i++) n = (n << 8) | (uint8_t)p[off + i];
		off += lenSize;
		if (off + n > total) break;
		[out appendBytes:startCode length:4];
		[out appendBytes:p + off length:n];
		off += n;
	}

	CMTime pts = CMSampleBufferGetPresentationTimeStamp(sb);
	int64_t ns = (int64_t)(CMTimeGetSeconds(pts) * 1e9);
	sgDeliverFrame(e->handle, (void *)out.bytes, (int)out.length, ns, key ? 1 : 0);
}

static OSStatus setProp(VTCompressionSessionRef s, CFStringRef key, CFTypeRef value, NSString **err) {
	OSStatus st = VTSessionSetProperty(s, key, value);
	if (st != noErr && err && !*err) *err = statusText([NSString stringWithFormat:@"set %@", (__bridge NSString *)key], st);
	return st;
}

sgEncoder *sgEncoderNew(int width, int height, int fps, int kbps, uintptr_t handle, char **err) {
	sgEncoder *e = calloc(1, sizeof(sgEncoder));
	e->handle = handle;
	e->width = width;
	e->height = height;

	NSDictionary *spec = @{(__bridge NSString *)kVTVideoEncoderSpecification_EnableHardwareAcceleratedVideoEncoder: @YES};
	OSStatus st = VTCompressionSessionCreate(NULL, width, height, kCMVideoCodecType_H264, (__bridge CFDictionaryRef)spec,
	                                         NULL, NULL, encoderOutput, e, &e->session);
	if (st != noErr) {
		*err = copyCString(statusText(@"VTCompressionSessionCreate", st));
		free(e);
		return NULL;
	}

	NSString *perr = nil;
	VTCompressionSessionRef s = e->session;
	setProp(s, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue, &perr);
	setProp(s, kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_ConstrainedBaseline_AutoLevel, &perr);
	setProp(s, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse, &perr);
	setProp(s, kVTCompressionPropertyKey_ExpectedFrameRate, (__bridge CFNumberRef)@(fps), &perr);
	setProp(s, kVTCompressionPropertyKey_AverageBitRate, (__bridge CFNumberRef)@(kbps * 1000), &perr);
	setProp(s, kVTCompressionPropertyKey_DataRateLimits, (__bridge CFArrayRef)@[@(kbps * 1000 / 8), @1.0], &perr);
	setProp(s, kVTCompressionPropertyKey_MaxKeyFrameIntervalDuration, (__bridge CFNumberRef)@10.0, &perr);
	setProp(s, kVTCompressionPropertyKey_ColorPrimaries, kCVImageBufferColorPrimaries_ITU_R_709_2, &perr);
	setProp(s, kVTCompressionPropertyKey_TransferFunction, kCVImageBufferTransferFunction_ITU_R_709_2, &perr);
	setProp(s, kVTCompressionPropertyKey_YCbCrMatrix, kCVImageBufferYCbCrMatrix_ITU_R_709_2, &perr);
	if (perr) {
		*err = copyCString(perr);
		sgEncoderFree(e);
		return NULL;
	}
	VTCompressionSessionPrepareToEncodeFrames(s);
	return e;
}

static OSStatus encodeBuffer(sgEncoder *e, CVPixelBufferRef pb, CMTime pts, BOOL forceKey) {
	NSDictionary *opts = forceKey ? @{(__bridge NSString *)kVTEncodeFrameOptionKey_ForceKeyFrame: @YES} : nil;
	return VTCompressionSessionEncodeFrame(e->session, pb, pts, kCMTimeInvalid, (__bridge CFDictionaryRef)opts, NULL, NULL);
}

int sgEncoderEncodeTestFrame(sgEncoder *e, int index, int64_t pts_ns) {
	CVPixelBufferRef pb = NULL;
	NSDictionary *attrs = @{(__bridge NSString *)kCVPixelBufferIOSurfacePropertiesKey: @{}};
	if (CVPixelBufferCreate(NULL, e->width, e->height, kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange,
	                        (__bridge CFDictionaryRef)attrs, &pb) != kCVReturnSuccess)
		return -1;
	CVPixelBufferLockBaseAddress(pb, 0);
	for (size_t plane = 0; plane < 2; plane++) {
		uint8_t *base = CVPixelBufferGetBaseAddressOfPlane(pb, plane);
		size_t stride = CVPixelBufferGetBytesPerRowOfPlane(pb, plane);
		size_t h = CVPixelBufferGetHeightOfPlane(pb, plane);
		for (size_t y = 0; y < h; y++)
			for (size_t x = 0; x < stride; x++) base[y * stride + x] = (uint8_t)(plane ? 128 : (x + y * 3 + index * 7));
	}
	CVPixelBufferUnlockBaseAddress(pb, 0);
	OSStatus st = encodeBuffer(e, pb, CMTimeMake(pts_ns, 1000000000), index == 0);
	CVPixelBufferRelease(pb);
	return (int)st;
}

void sgEncoderFree(sgEncoder *e) {
	if (!e) return;
	if (e->session) {
		VTCompressionSessionCompleteFrames(e->session, kCMTimeInvalid);
		VTCompressionSessionInvalidate(e->session);
		CFRelease(e->session);
	}
	free(e);
}

// ---- Capture: ScreenCaptureKit display stream feeding the encoder ----

@interface SGCapture : NSObject <SCStreamOutput, SCStreamDelegate>
@property(nonatomic) sgEncoder *encoder;
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) SCStream *stream;
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic) CVPixelBufferRef last;
@property(nonatomic) BOOL stopped;
@end

@implementation SGCapture

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sb ofType:(SCStreamOutputType)type {
	if (type != SCStreamOutputTypeScreen || self.stopped) return;
	CFArrayRef atts = CMSampleBufferGetSampleAttachmentsArray(sb, false);
	if (!atts || CFArrayGetCount(atts) == 0) return;
	NSDictionary *info = (__bridge NSDictionary *)CFArrayGetValueAtIndex(atts, 0);
	NSNumber *status = info[SCStreamFrameInfoStatus];
	if (status == nil || status.integerValue != SCFrameStatusComplete) return;
	CVPixelBufferRef pb = CMSampleBufferGetImageBuffer(sb);
	if (!pb) return;
	CVPixelBufferRetain(pb);
	if (self.last) CVPixelBufferRelease(self.last);
	self.last = pb;
	OSStatus st = encodeBuffer(self.encoder, pb, CMSampleBufferGetPresentationTimeStamp(sb), NO);
	if (st != noErr) [self fail:statusText(@"VTCompressionSessionEncodeFrame", st)];
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
	if (!self.stopped) [self fail:[NSString stringWithFormat:@"screen capture stopped: %@", error.localizedDescription]];
}

- (void)fail:(NSString *)msg {
	char *m = copyCString(msg);
	sgDeliverError(self.handle, m);
	free(m);
}

// Re-encodes the last captured picture as a keyframe, so a lost keyframe is
// recovered even when the screen does not change.
- (void)keyframe {
	if (self.stopped || !self.last) return;
	CMTime now = CMClockGetTime(CMClockGetHostTimeClock());
	encodeBuffer(self.encoder, self.last, now, YES);
}

@end

struct sgCapture {
	void *obj; // SGCapture, retained
};

static SCDisplay *findDisplay(uint32_t displayID, NSString **err) {
	// A display created a moment ago can take a little while to be listed.
	for (int attempt = 0; attempt < 20; attempt++) {
		__block SCShareableContent *content = nil;
		__block NSError *e = nil;
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		[SCShareableContent getShareableContentExcludingDesktopWindows:NO
		                                           onScreenWindowsOnly:YES
		                                             completionHandler:^(SCShareableContent *c, NSError *error) {
			                                             content = c;
			                                             e = error;
			                                             dispatch_semaphore_signal(done);
		                                             }];
		if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC)) != 0) {
			*err = @"ScreenCaptureKit did not answer (Screen Recording permission?)";
			return nil;
		}
		if (e) {
			*err = [NSString stringWithFormat:@"screen capture not allowed: %@", e.localizedDescription];
			return nil;
		}
		for (SCDisplay *d in content.displays)
			if (d.displayID == displayID) return d;
		usleep(150 * 1000);
	}
	*err = [NSString stringWithFormat:@"display %u is not available to ScreenCaptureKit", displayID];
	return nil;
}

static void stopCapture(SGCapture *c);

sgCapture *sgCaptureStart(uint32_t displayID, int width, int height, int fps, int kbps, uintptr_t handle, char **err) {
	@autoreleasepool {
		NSString *e = nil;
		SCDisplay *display = findDisplay(displayID, &e);
		if (!display) {
			*err = copyCString(e);
			return NULL;
		}
		size_t w = (size_t)width, h = (size_t)height;

		SGCapture *c = [SGCapture new];
		c.handle = handle;
		c.queue = dispatch_queue_create("spareglass.capture", DISPATCH_QUEUE_SERIAL);
		c.encoder = sgEncoderNew((int)w, (int)h, fps, kbps, handle, err);
		if (!c.encoder) return NULL;

		SCStreamConfiguration *cfg = [SCStreamConfiguration new];
		cfg.width = w;
		cfg.height = h;
		cfg.minimumFrameInterval = CMTimeMake(1, fps);
		// BGRA, so VideoToolbox converts to YUV: ScreenCaptureKit's own YUV output
		// comes out about 8/255 brighter than the screen.
		cfg.pixelFormat = kCVPixelFormatType_32BGRA;
		cfg.showsCursor = YES;
		cfg.queueDepth = 3;

		SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
		c.stream = [[SCStream alloc] initWithFilter:filter configuration:cfg delegate:c];
		NSError *addErr = nil;
		if (![c.stream addStreamOutput:c type:SCStreamOutputTypeScreen sampleHandlerQueue:c.queue error:&addErr]) {
			*err = copyCString([NSString stringWithFormat:@"add stream output: %@", addErr.localizedDescription]);
			sgEncoderFree(c.encoder);
			return NULL;
		}
		__block NSError *startErr = nil;
		dispatch_semaphore_t started = dispatch_semaphore_create(0);
		[c.stream startCaptureWithCompletionHandler:^(NSError *error) {
			startErr = error;
			dispatch_semaphore_signal(started);
		}];
		if (dispatch_semaphore_wait(started, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC)) != 0 || startErr) {
			*err = copyCString([NSString stringWithFormat:@"start screen capture: %@",
			                                               startErr ? startErr.localizedDescription : @"timed out"]);
			stopCapture(c);
			return NULL;
		}
		sgCapture *out = calloc(1, sizeof(sgCapture));
		out->obj = (__bridge_retained void *)c;
		return out;
	}
}

void sgCaptureRequestKeyframe(sgCapture *cap) {
	SGCapture *c = (__bridge SGCapture *)cap->obj;
	dispatch_async(c.queue, ^{
		[c keyframe];
	});
}

void sgCaptureStop(sgCapture *cap) {
	@autoreleasepool {
		SGCapture *c = (__bridge_transfer SGCapture *)cap->obj;
		free(cap);
		stopCapture(c);
	}
}

// stopCapture stops the stream, waits for queued sample handlers, then frees
// the encoder, so no callback reaches a freed encoder or a deleted handle.
static void stopCapture(SGCapture *c) {
	dispatch_semaphore_t stopped = dispatch_semaphore_create(0);
	[c.stream stopCaptureWithCompletionHandler:^(NSError *error) {
		dispatch_semaphore_signal(stopped);
	}];
	dispatch_semaphore_wait(stopped, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
	// Wait for any sample handler or keyframe request already queued.
	dispatch_sync(c.queue, ^{
		c.stopped = YES;
	});
	sgEncoderFree(c.encoder);
	c.encoder = NULL;
	if (c.last) CVPixelBufferRelease(c.last);
	c.last = NULL;
}

// ---- Pointer position ----

int sgCursorPosition(uint32_t display_id, double *x, double *y) {
	CGEventRef e = CGEventCreate(NULL);
	if (!e) return 0;
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	CGRect b = CGDisplayBounds(display_id);
	*x = p.x - b.origin.x;
	*y = p.y - b.origin.y;
	return CGRectContainsPoint(b, p) ? 1 : 0;
}
