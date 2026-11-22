// mover: post kCGEventMouseMoved to the HID event tap.
//   mover sweep DISPLAY_ID SECONDS [HZ]   move along a path at HZ (default 120)
//   mover at DISPLAY_ID X Y               move to X,Y points inside the display
//   mover where                           print the cursor location (global points)
#include <ApplicationServices/ApplicationServices.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <mach/mach_time.h>

static void post(CGPoint p) {
	CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, p, kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

int main(int argc, char **argv) {
	if (argc >= 2 && strcmp(argv[1], "where") == 0) {
		CGEventRef e = CGEventCreate(NULL);
		CGPoint p = CGEventGetLocation(e);
		CFRelease(e);
		printf("%.1f %.1f\n", p.x, p.y);
		return 0;
	}
	if (argc < 4) {
		fprintf(stderr, "usage: mover sweep ID SECONDS [HZ] | at ID X Y | where\n");
		return 2;
	}
	CGDirectDisplayID id = (CGDirectDisplayID)strtoul(argv[2], NULL, 10);
	CGRect b = CGDisplayBounds(id);
	if (b.size.width == 0) {
		fprintf(stderr, "display %u not found\n", id);
		return 1;
	}
	if (strcmp(argv[1], "at") == 0) {
		if (argc < 5) {
			fprintf(stderr, "usage: mover at ID X Y\n");
			return 2;
		}
		post(CGPointMake(b.origin.x + atof(argv[3]), b.origin.y + atof(argv[4])));
		return 0;
	}
	if (strcmp(argv[1], "sweep") != 0) {
		fprintf(stderr, "unknown command %s\n", argv[1]);
		return 2;
	}
	double secs = atof(argv[3]);
	double hz = argc >= 5 ? atof(argv[4]) : 120;
	mach_timebase_info_data_t tb;
	mach_timebase_info(&tb);
	uint64_t step = (uint64_t)(1e9 / hz) * tb.denom / tb.numer;
	uint64_t t = mach_absolute_time();
	long n = (long)(secs * hz);
	for (long i = 0; i < n; i++) {
		double u = (double)(i % 240) / 240;
		post(CGPointMake(b.origin.x + b.size.width * (0.15 + 0.7 * u),
		                 b.origin.y + b.size.height * (0.5 + 0.3 * sin(u * 2 * M_PI))));
		t += step;
		mach_wait_until(t);
	}
	printf("%ld events\n", n);
	return 0;
}
