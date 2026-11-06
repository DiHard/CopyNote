#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <WebKit/WebKit.h>
#import <Carbon/Carbon.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include "macui.h"
#include "_cgo_export.h"

// The window anchors under the menu bar icon, this far from the screen edges.
static const CGFloat kMargin = 8;
static const CGFloat kMinHeight = 80;
static const CGFloat kCornerRadius = 10;

@interface CNWindow : NSWindow
@end

@implementation CNWindow
// A borderless window refuses key status unless told otherwise.
- (BOOL)canBecomeKeyWindow { return YES; }
- (BOOL)canBecomeMainWindow { return YES; }
@end

@interface CNController : NSObject <NSApplicationDelegate, WKScriptMessageHandler, WKNavigationDelegate, NSMenuDelegate>
@property(strong) CNWindow *window;
@property(strong) WKWebView *webView;
@property(strong) NSStatusItem *statusItem;
@property(strong) NSMenu *openMenu;
@property CGFloat contentHeight;
@property BOOL loaded;
@end

static CNController *controller;
static EventHotKeyRef hotkeyRef;
static EventHandlerRef hotkeyHandler;

// The Lucide "copy" glyph the Windows tray icon is drawn from, as a template
// image: macOS tints it for the menu bar's light or dark appearance.
static NSImage *statusIcon(void) {
	NSImage *image = [NSImage imageWithSize:NSMakeSize(18, 18) flipped:YES drawingHandler:^BOOL(NSRect rect) {
		CGFloat s = 18.0 / 24.0;
		NSAffineTransform *t = [NSAffineTransform transform];
		[t scaleBy:s];
		[t concat];
		[[NSColor blackColor] setStroke];
		NSBezierPath *front = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(8, 8, 14, 14) xRadius:2 yRadius:2];
		front.lineWidth = 2;
		[front stroke];
		NSBezierPath *back = [NSBezierPath bezierPath];
		back.lineWidth = 2;
		back.lineCapStyle = NSLineCapStyleRound;
		back.lineJoinStyle = NSLineJoinStyleRound;
		[back moveToPoint:NSMakePoint(4, 16)];
		[back curveToPoint:NSMakePoint(2, 14) controlPoint1:NSMakePoint(2.9, 16) controlPoint2:NSMakePoint(2, 15.1)];
		[back lineToPoint:NSMakePoint(2, 4)];
		[back curveToPoint:NSMakePoint(4, 2) controlPoint1:NSMakePoint(2, 2.9) controlPoint2:NSMakePoint(2.9, 2)];
		[back lineToPoint:NSMakePoint(14, 2)];
		[back curveToPoint:NSMakePoint(16, 4) controlPoint1:NSMakePoint(15.1, 2) controlPoint2:NSMakePoint(16, 2.9)];
		[back stroke];
		return YES;
	}];
	image.template = YES;
	return image;
}

// The menu bar is invisible for an accessory app, but its key equivalents
// still work, and WKWebView's text fields only cut, copy and paste through
// them.
static void installMainMenu(void) {
	NSMenu *bar = [[NSMenu alloc] init];
	NSMenuItem *appItem = [[NSMenuItem alloc] init];
	NSMenu *appMenu = [[NSMenu alloc] init];
	[appMenu addItemWithTitle:@"Quit CopyNote" action:@selector(terminate:) keyEquivalent:@"q"];
	appItem.submenu = appMenu;
	[bar addItem:appItem];

	NSMenuItem *editItem = [[NSMenuItem alloc] init];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
	NSMenuItem *redo = [edit addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"z"];
	redo.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
	editItem.submenu = edit;
	[bar addItem:editItem];
	NSApp.mainMenu = bar;
}

static OSStatus onHotkey(EventHandlerCallRef next, EventRef event, void *data) {
	goHotkey();
	return noErr;
}

static void activateApp(void) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	// The cooperative [NSApp activate] of macOS 14 may decline a request
	// that came from a global hotkey; the window would then show without
	// the keyboard.
	[NSApp activateIgnoringOtherApps:YES];
#pragma clang diagnostic pop
}

@implementation CNController

- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
	// Opening CopyNote.app while it runs: LaunchServices activates this
	// copy instead of starting a second one.
	goReopen();
	return NO;
}

- (void)applicationDidResignActive:(NSNotification *)notification {
	goResignActive();
}

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
	// Cmd+Q and the menu's Quit end the run loop the same way; main then
	// shuts down like on any other quit.
	goQuitRequested();
	return NSTerminateCancel;
}

- (void)userContentController:(WKUserContentController *)ucc didReceiveScriptMessage:(WKScriptMessage *)message {
	if (![message.body isKindOfClass:[NSString class]]) {
		return;
	}
	NSString *body = message.body;
	// Answered from a block of its own rather than from inside WebKit's
	// callback, so a binding may open a modal panel or a menu.
	dispatch_async(dispatch_get_main_queue(), ^{
		goScriptMessage((char *)body.UTF8String);
	});
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action
	decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	// The page is the one HTML file loaded at start. A file dropped on the
	// window, a link or a reload — which would fetch the page's made-up
	// origin — must not replace it.
	if (!self.loaded) {
		self.loaded = YES;
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	decisionHandler(WKNavigationActionPolicyCancel);
}

- (void)statusItemClicked:(id)sender {
	NSEvent *event = NSApp.currentEvent;
	BOOL right = event.type == NSEventTypeRightMouseUp || (event.modifierFlags & NSEventModifierFlagControl);
	goStatusClick(right ? 1 : 0);
}

- (void)menuPicked:(NSMenuItem *)item {
	NSMenu *menu = item.menu;
	menu.delegate = nil; // menuDidClose must not answer a second time
	if (self.openMenu == menu) {
		self.openMenu = nil;
	}
	goMenuClosed((int)menu.identifier.integerValue, (int)item.tag);
}

- (void)menuDidClose:(NSMenu *)menu {
	// menuDidClose arrives before the picked item's action; wait for a
	// turn of the run loop to tell a dismissal from a pick.
	dispatch_async(dispatch_get_main_queue(), ^{
		if (menu.delegate == nil) {
			return;
		}
		menu.delegate = nil;
		if (self.openMenu == menu) {
			self.openMenu = nil;
		}
		goMenuClosed((int)menu.identifier.integerValue, -1);
	});
}

@end

void macuiInit(int width, int height) {
	[NSApplication sharedApplication];
	// No Dock icon and no app switcher entry, like WS_EX_TOOLWINDOW on
	// Windows. Info.plist says the same with LSUIElement; this covers a
	// binary started outside the bundle.
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	controller = [[CNController alloc] init];
	NSApp.delegate = controller;
	installMainMenu();

	CNWindow *window = [[CNWindow alloc] initWithContentRect:NSMakeRect(0, 0, width, height)
		styleMask:NSWindowStyleMaskBorderless backing:NSBackingStoreBuffered defer:NO];
	window.opaque = NO;
	window.backgroundColor = NSColor.clearColor;
	window.hasShadow = YES;
	window.releasedWhenClosed = NO;
	window.level = NSFloatingWindowLevel;
	window.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
		NSWindowCollectionBehaviorFullScreenAuxiliary | NSWindowCollectionBehaviorIgnoresCycle;

	NSView *content = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, height)];
	content.wantsLayer = YES;
	content.layer.cornerRadius = kCornerRadius;
	content.layer.masksToBounds = YES;
	window.contentView = content;

	WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
	[config.userContentController addScriptMessageHandler:controller name:@"copynote"];
	WKWebView *webView = [[WKWebView alloc] initWithFrame:content.bounds configuration:config];
	webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	webView.navigationDelegate = controller;
	webView.allowsMagnification = NO;
	webView.allowsBackForwardNavigationGestures = NO;
	// The page paints its own background; the corners outside it stay clear.
	[webView setValue:@NO forKey:@"drawsBackground"];
	if (@available(macOS 13.3, *)) {
		// Safari's Develop menu can attach to the page when this is set.
		webView.inspectable = getenv("COPYNOTE_INSPECT") != NULL;
	}
	[content addSubview:webView];

	controller.window = window;
	controller.webView = webView;
	controller.contentHeight = height;

	NSStatusItem *item = [NSStatusBar.systemStatusBar statusItemWithLength:NSSquareStatusItemLength];
	item.button.image = statusIcon();
	item.button.target = controller;
	item.button.action = @selector(statusItemClicked:);
	[item.button sendActionOn:NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp];
	controller.statusItem = item;

	EventTypeSpec spec = {kEventClassKeyboard, kEventHotKeyPressed};
	InstallApplicationEventHandler(&onHotkey, 1, &spec, NULL, &hotkeyHandler);
}

void macuiAddUserScript(const char *js) {
	WKUserScript *script = [[WKUserScript alloc] initWithSource:[NSString stringWithUTF8String:js]
		injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
	[controller.webView.configuration.userContentController addUserScript:script];
}

void macuiLoadHTML(const char *html, const char *baseURL) {
	[controller.webView loadHTMLString:[NSString stringWithUTF8String:html]
		baseURL:[NSURL URLWithString:[NSString stringWithUTF8String:baseURL]]];
}

void macuiRun(void) {
	[NSApp run];
}

void macuiStop(void) {
	[NSApp stop:nil];
	// stop: takes effect after the next event, which may be a long time
	// coming in an app that is sitting in the menu bar.
	NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint
		modifierFlags:0 timestamp:0 windowNumber:0 context:nil subtype:0 data1:0 data2:0];
	[NSApp postEvent:wake atStart:NO];
}

void macuiDispatch(uintptr_t id) {
	// The common modes include the one a context menu runs the loop in
	// while it is open; the main dispatch queue waits until it closes, and
	// a hotkey or a timer would wait with it.
	CFRunLoopRef main = CFRunLoopGetMain();
	CFRunLoopPerformBlock(main, kCFRunLoopCommonModes, ^{
		goDispatch(id);
	});
	CFRunLoopWakeUp(main);
}

void macuiEval(const char *js) {
	[controller.webView evaluateJavaScript:[NSString stringWithUTF8String:js] completionHandler:nil];
}

// The screen area the window lives in and the right edge to align it to:
// under the menu bar icon, or the top-right corner when the icon has no
// window (hidden behind the notch or by a menu bar manager).
static void anchor(NSRect *visible, CGFloat *right) {
	NSWindow *iconWindow = controller.statusItem.button.window;
	NSScreen *screen = iconWindow.screen ?: NSScreen.mainScreen;
	*visible = screen.visibleFrame;
	*right = NSMaxX(*visible) - kMargin;
	if (iconWindow != nil && iconWindow.screen != nil) {
		*right = MIN(NSMaxX(iconWindow.frame), *right);
	}
}

static void placeWindow(void) {
	NSRect visible;
	CGFloat right;
	anchor(&visible, &right);
	CGFloat width = controller.window.frame.size.width;
	CGFloat height = MIN(MAX(controller.contentHeight, kMinHeight), visible.size.height - 2 * kMargin);
	CGFloat x = MAX(right - width, NSMinX(visible) + kMargin);
	CGFloat top = NSMaxY(visible) - kMargin;
	[controller.window setFrame:NSMakeRect(x, top - height, width, height) display:YES];
	[controller.window invalidateShadow];
}

// setWindowAlpha sets the window's opacity over duration seconds; 0 also
// stops a fade that is still running.
static void setWindowAlpha(CGFloat alpha, NSTimeInterval duration) {
	[NSAnimationContext runAnimationGroup:^(NSAnimationContext *context) {
		context.duration = duration;
		context.timingFunction = [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseOut];
		controller.window.animator.alphaValue = alpha;
	} completionHandler:nil];
}

void macuiShowWindow(void) {
	NSWindow *window = controller.window;
	placeWindow();
	activateApp();
	if (!window.visible) {
		// Fades in the way the menu bar's own panels do. The page is already
		// prepared for this show, so nothing changes underneath the fade.
		window.alphaValue = 0;
		[window makeKeyAndOrderFront:nil];
		setWindowAlpha(1, NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion ? 0 : 0.12);
	} else {
		[window makeKeyAndOrderFront:nil];
	}
	[window makeFirstResponder:controller.webView];
}

void macuiHideWindow(int giveBackFocus) {
	// Gone at once, like a menu: the page resets itself right after, and a
	// fade-out would show that happening.
	setWindowAlpha(1, 0);
	[controller.window orderOut:nil];
	if (giveBackFocus && NSApp.active) {
		// Hands the keyboard back to the app the user came from, as Cmd+H
		// does; the window is already out of the way.
		[NSApp hide:nil];
	}
}

void macuiFocusWindow(void) {
	activateApp();
	[controller.window makeKeyAndOrderFront:nil];
}

void macuiSetContentHeight(int height) {
	controller.contentHeight = height;
	if (controller.window.visible) {
		placeWindow(); // keeps the top edge under the menu bar
	}
}

int macuiWindowKey(void) {
	return NSApp.active && controller.window.visible && controller.window.keyWindow;
}

void macuiSetTopmost(int topmost) {
	controller.window.level = topmost ? NSFloatingWindowLevel : NSNormalWindowLevel;
}

void macuiSetStatusTooltip(const char *text) {
	controller.statusItem.button.toolTip = [NSString stringWithUTF8String:text];
}

int macuiAlert(const char *title, const char *text, const char *first, const char *second) {
	activateApp();
	NSAlert *alert = [[NSAlert alloc] init];
	alert.messageText = [NSString stringWithUTF8String:title];
	alert.informativeText = [NSString stringWithUTF8String:text];
	[alert addButtonWithTitle:[NSString stringWithUTF8String:first]];
	[alert addButtonWithTitle:[NSString stringWithUTF8String:second]];
	return [alert runModal] == NSAlertFirstButtonReturn ? 0 : 1;
}

void macuiShowMenu(int menuID, const char **labels, const char **keys, const unsigned long *keyMods,
                   const int *flags, int count, double x, double y, int atStatusItem, int appearance) {
	NSMenu *menu = [[NSMenu alloc] init];
	menu.autoenablesItems = NO;
	menu.identifier = [NSString stringWithFormat:@"%d", menuID];
	if (appearance == 1) {
		menu.appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
	} else if (appearance == 2) {
		menu.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
	}
	for (int i = 0; i < count; i++) {
		if (flags[i] & 2) {
			[menu addItem:[NSMenuItem separatorItem]];
			continue;
		}
		NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:[NSString stringWithUTF8String:labels[i]]
			action:@selector(menuPicked:) keyEquivalent:[NSString stringWithUTF8String:keys[i]]];
		item.keyEquivalentModifierMask = keyMods[i];
		item.target = controller;
		item.tag = i;
		item.enabled = !(flags[i] & 1);
		[menu addItem:item];
	}
	menu.delegate = controller;
	[controller.openMenu cancelTrackingWithoutAnimation];
	controller.openMenu = menu;
	// Opened from a block of its own: popUpMenu tracks the mouse until the
	// menu closes, and the caller is waiting to return.
	dispatch_async(dispatch_get_main_queue(), ^{
		if (controller.openMenu != menu) {
			// Replaced before it opened, so it never closes either.
			menu.delegate = nil;
			goMenuClosed(menuID, -1);
			return;
		}
		if (atStatusItem) {
			NSStatusBarButton *button = controller.statusItem.button;
			[menu popUpMenuPositioningItem:nil atLocation:NSMakePoint(0, NSMaxY(button.bounds) + 4) inView:button];
		} else {
			[menu popUpMenuPositioningItem:nil atLocation:NSMakePoint(x, y) inView:controller.webView];
		}
	});
}

void macuiCancelMenu(void) {
	[controller.openMenu cancelTrackingWithoutAnimation];
}

int macuiRegisterHotkey(uint32_t keyCode, uint32_t modifiers) {
	EventHotKeyID id = {'CpNt', 1};
	return RegisterEventHotKey(keyCode, modifiers, id, GetApplicationEventTarget(), 0, &hotkeyRef);
}

void macuiUnregisterHotkey(void) {
	if (hotkeyRef != NULL) {
		UnregisterEventHotKey(hotkeyRef);
		hotkeyRef = NULL;
	}
}

static char *pathOf(NSURL *url) {
	return url == nil ? NULL : strdup(url.path.fileSystemRepresentation);
}

char *macuiSavePanel(const char *defaultName) {
	NSSavePanel *panel = [NSSavePanel savePanel];
	panel.nameFieldStringValue = [NSString stringWithUTF8String:defaultName];
	panel.allowedContentTypes = @[ UTTypeJSON ];
	activateApp();
	return [panel runModal] == NSModalResponseOK ? pathOf(panel.URL) : NULL;
}

char *macuiOpenPanel(void) {
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	panel.allowedContentTypes = @[ UTTypeJSON ];
	panel.allowsMultipleSelection = NO;
	activateApp();
	return [panel runModal] == NSModalResponseOK ? pathOf(panel.URLs.firstObject) : NULL;
}

void macuiEvalResult(int callID, const char *js) {
	[controller.webView evaluateJavaScript:[NSString stringWithUTF8String:js] completionHandler:^(id result, NSError *error) {
		NSString *text = error ? [@"error: " stringByAppendingString:error.localizedDescription] : [NSString stringWithFormat:@"%@", result];
		goEvalResult(callID, (char *)text.UTF8String);
	}];
}

void macuiSnapshot(int callID, const char *path) {
	NSString *target = [NSString stringWithUTF8String:path];
	[controller.webView takeSnapshotWithConfiguration:nil completionHandler:^(NSImage *image, NSError *error) {
		NSString *text = @"ok";
		if (image == nil) {
			text = [@"error: " stringByAppendingString:error.localizedDescription];
		} else {
			NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithData:image.TIFFRepresentation];
			NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
			if (![png writeToFile:target atomically:YES]) {
				text = @"error: write failed";
			}
		}
		goEvalResult(callID, (char *)text.UTF8String);
	}];
}
