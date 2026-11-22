// flash: show a randomly colored square on one display for a few seconds, so
// the next captured frame of that display must differ.
//   flash DISPLAY_ID [SECONDS]
// It prints "shown" once the window is on screen.
#import <AppKit/AppKit.h>

int main(int argc, char **argv) {
	@autoreleasepool {
		if (argc < 2) {
			fprintf(stderr, "usage: flash DISPLAY_ID [SECONDS]\n");
			return 2;
		}
		unsigned long want = strtoul(argv[1], NULL, 10);
		double secs = argc > 2 ? atof(argv[2]) : 3;
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		NSScreen *screen = nil;
		for (NSScreen *s in [NSScreen screens]) {
			if ([s.deviceDescription[@"NSScreenNumber"] unsignedLongValue] == want) screen = s;
		}
		if (!screen) {
			fprintf(stderr, "display %lu not found\n", want);
			return 1;
		}
		NSRect f = screen.frame;
		NSRect box = NSMakeRect(NSMidX(f) - 150, NSMidY(f) - 150, 300, 300);
		NSWindow *w = [[NSWindow alloc] initWithContentRect:box styleMask:NSWindowStyleMaskBorderless backing:NSBackingStoreBuffered defer:NO];
		w.level = NSStatusWindowLevel;
		w.backgroundColor = [NSColor colorWithRed:arc4random_uniform(256) / 255.0 green:arc4random_uniform(256) / 255.0 blue:arc4random_uniform(256) / 255.0 alpha:1];
		[w orderFrontRegardless];
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.3]];
		printf("shown\n");
		fflush(stdout);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:secs]];
	}
	return 0;
}
