// The Cocoa side of the macOS host. Every function here runs on the main
// thread; macui.go is what makes sure of that.
#include <stdint.h>

void macuiInit(int width, int height);
void macuiAddUserScript(const char *js);
void macuiLoadHTML(const char *html, const char *baseURL);
void macuiRun(void);
void macuiStop(void);
void macuiDispatch(uintptr_t id);
void macuiEval(const char *js);

void macuiShowWindow(void);
void macuiHideWindow(int giveBackFocus);
void macuiFocusWindow(void);
void macuiSetContentHeight(int height);
int macuiWindowKey(void);
void macuiSetTopmost(int topmost);

void macuiSetStatusTooltip(const char *text);
int macuiAlert(const char *title, const char *text, const char *first, const char *second);

// flags: 1 = disabled, 2 = separator. appearance: 0 system, 1 light, 2 dark.
// x and y are points in the web view, from its top-left corner; atStatusItem
// opens the menu under the menu bar icon instead.
void macuiShowMenu(int menuID, const char **labels, const char **keys, const unsigned long *keyMods,
                   const int *flags, int count, double x, double y, int atStatusItem, int appearance);
void macuiCancelMenu(void);

int macuiRegisterHotkey(uint32_t keyCode, uint32_t modifiers);
void macuiUnregisterHotkey(void);

// The chosen path, malloc'd for the caller to free, or NULL when cancelled.
char *macuiSavePanel(const char *defaultName);
char *macuiOpenPanel(void);

// Development aids for the devtools build (see devtools_darwin.go): evaluate
// js and report its result as a string, and save the page as a PNG.
void macuiEvalResult(int callID, const char *js);
void macuiSnapshot(int callID, const char *path);
