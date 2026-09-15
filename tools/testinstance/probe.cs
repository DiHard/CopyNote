using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;

// The Win32 side of the test-instance checks, compiled by Add-Type in common.ps1.
public static class CopyNoteProbe
{
    delegate bool EnumProc(IntPtr hwnd, IntPtr lParam);

    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }

    [StructLayout(LayoutKind.Sequential)]
    struct MSG { public IntPtr hwnd; public uint message; public IntPtr wParam; public IntPtr lParam; public uint time; public int ptX; public int ptY; }

    [DllImport("user32.dll")] static extern IntPtr SetThreadDpiAwarenessContext(IntPtr value);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc callback, IntPtr lParam);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowTextW(IntPtr hwnd, StringBuilder text, int max);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr hwnd, out RECT rect);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern IntPtr FindWindowExW(IntPtr parent, IntPtr after, string className, string title);
    [DllImport("user32.dll")] static extern bool PostMessageW(IntPtr hwnd, uint msg, IntPtr wParam, IntPtr lParam);
    [DllImport("user32.dll", SetLastError = true)] static extern bool RegisterHotKey(IntPtr hwnd, int id, uint mods, uint vk);
    [DllImport("user32.dll")] static extern bool UnregisterHotKey(IntPtr hwnd, int id);
    [DllImport("user32.dll")] static extern void keybd_event(byte vk, byte scan, uint flags, UIntPtr extra);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool AllowSetForegroundWindow(int pid);
    [DllImport("user32.dll")] static extern bool PeekMessageW(out MSG msg, IntPtr hwnd, uint min, uint max, uint remove);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern IntPtr FindWindowW(string className, string title);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassNameW(IntPtr hwnd, StringBuilder name, int max);
    [DllImport("user32.dll")] static extern IntPtr GetWindow(IntPtr hwnd, uint cmd);
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr hwnd);
    [DllImport("user32.dll")] static extern uint GetDpiForWindow(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool AttachThreadInput(uint attach, uint attachTo, bool on);
    [DllImport("user32.dll")] static extern bool BringWindowToTop(IntPtr hwnd);
    [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();

    static readonly IntPtr HwndMessage = new IntPtr(-3);

    // The app parks a hidden window at -30000 and creates it off-screen too.
    const int ParkedLeftOf = -5000;

    public static long Now() { return DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(); }

    // The message-only tray window of the given class, or zero.
    public static IntPtr Tray(string className)
    {
        return FindWindowExW(HwndMessage, IntPtr.Zero, className, null);
    }

    // The visible top-level "CopyNote" window owned by pid, or zero.
    public static IntPtr MainWindow(int pid)
    {
        IntPtr found = IntPtr.Zero;
        EnumWindows((h, l) =>
        {
            uint owner;
            GetWindowThreadProcessId(h, out owner);
            if ((int)owner != pid || !IsWindowVisible(h)) return true;
            var title = new StringBuilder(64);
            GetWindowTextW(h, title, title.Capacity);
            if (title.ToString() != "CopyNote") return true;
            found = h;
            return false;
        }, IntPtr.Zero);
        return found;
    }

    // In physical pixels, whatever the DPI awareness of the calling shell.
    public static RECT Rect(IntPtr hwnd)
    {
        SetThreadDpiAwarenessContext(new IntPtr(-4));
        RECT r;
        GetWindowRect(hwnd, out r);
        return r;
    }

    // A missing window counts as parked: nothing is on screen.
    public static bool Parked(IntPtr hwnd)
    {
        return hwnd == IntPtr.Zero || Rect(hwnd).Left <= ParkedLeftOf;
    }

    public static string Where(IntPtr hwnd)
    {
        if (hwnd == IntPtr.Zero) return "no window";
        var r = Rect(hwnd);
        if (r.Left <= ParkedLeftOf) return string.Format("parked at {0},{1}", r.Left, r.Top);
        return string.Format("on screen at {0},{1}, {2}x{3}", r.Left, r.Top, r.Right - r.Left, r.Bottom - r.Top);
    }

    // 0 when nobody holds the combination; otherwise the error RegisterHotKey
    // reported, 1409 meaning another program holds it.
    public static int HotkeyState(uint mods, uint vk)
    {
        if (RegisterHotKey(IntPtr.Zero, 0xB0B, mods, vk))
        {
            UnregisterHotKey(IntPtr.Zero, 0xB0B);
            return 0;
        }
        return Marshal.GetLastWin32Error();
    }

    // Types the combination for real. mods: 1 Alt, 2 Ctrl, 4 Shift, 8 Win.
    public static void Press(uint mods, byte vk)
    {
        const uint keyUp = 2;
        var held = new List<byte>();
        if ((mods & 2) != 0) held.Add(0x11);
        if ((mods & 1) != 0) held.Add(0x12);
        if ((mods & 4) != 0) held.Add(0x10);
        if ((mods & 8) != 0) held.Add(0x5B);
        foreach (var key in held) keybd_event(key, 0, 0, UIntPtr.Zero);
        keybd_event(vk, 0, 0, UIntPtr.Zero);
        keybd_event(vk, 0, keyUp, UIntPtr.Zero);
        for (int i = held.Count - 1; i >= 0; i--) keybd_event(held[i], 0, keyUp, UIntPtr.Zero);
    }

    public static bool Post(IntPtr hwnd, uint msg, long wParam, long lParam)
    {
        return PostMessageW(hwnd, msg, new IntPtr(wParam), new IntPtr(lParam));
    }

    public static IntPtr Foreground() { return GetForegroundWindow(); }

    // The visible popup menu pid shows - internal/popupmenu's window - or zero.
    public static IntPtr Menu(int pid)
    {
        IntPtr found = IntPtr.Zero;
        EnumWindows((h, l) =>
        {
            uint owner;
            GetWindowThreadProcessId(h, out owner);
            if ((int)owner != pid || !IsWindowVisible(h)) return true;
            var name = new StringBuilder(64);
            GetClassNameW(h, name, name.Capacity);
            if (name.ToString() != "CopyNotePopupMenu") return true;
            found = h;
            return false;
        }, IntPtr.Zero);
        return found;
    }

    public static IntPtr WaitMenu(int pid, int timeoutMs)
    {
        long deadline = Environment.TickCount64 + timeoutMs;
        while (Environment.TickCount64 < deadline)
        {
            IntPtr menu = Menu(pid);
            if (menu != IntPtr.Zero) return menu;
            Thread.Sleep(5);
        }
        return IntPtr.Zero;
    }

    public static bool WaitMenuGone(int pid, int timeoutMs)
    {
        long deadline = Environment.TickCount64 + timeoutMs;
        while (Environment.TickCount64 < deadline)
        {
            if (Menu(pid) == IntPtr.Zero) return true;
            Thread.Sleep(5);
        }
        return false;
    }

    // The window hwnd belongs to (GW_OWNER), or zero.
    public static IntPtr Owner(IntPtr hwnd) { return GetWindow(hwnd, 4); }

    public static uint Dpi(IntPtr hwnd) { return GetDpiForWindow(hwnd); }

    public static IntPtr Taskbar() { return FindWindowW("Shell_TrayWnd", null); }

    // Makes hwnd the foreground window, as a click into it would, even where
    // Windows refuses this process: for the one call, the thread shares the
    // input state of the thread that has the foreground now. Test tooling
    // only - an application must never do this.
    public static bool ForceForeground(IntPtr hwnd)
    {
        uint pid;
        uint foregroundThread = GetWindowThreadProcessId(GetForegroundWindow(), out pid);
        uint self = GetCurrentThreadId();
        bool attached = foregroundThread != 0 && foregroundThread != self && AttachThreadInput(self, foregroundThread, true);
        SetForegroundWindow(hwnd);
        BringWindowToTop(hwnd);
        if (attached) AttachThreadInput(self, foregroundThread, false);
        return GetForegroundWindow() == hwnd;
    }

    // Waits for the tray window, then posts msg to it count times at once.
    // Returns the Unix ms of the post, or -1 on timeout.
    public static long PostWhenTrayAppears(string className, uint msg, long wParam, long lParam, int count, int timeoutMs)
    {
        long deadline = Environment.TickCount64 + timeoutMs;
        while (Environment.TickCount64 < deadline)
        {
            IntPtr tray = Tray(className);
            if (tray != IntPtr.Zero)
            {
                long at = Now();
                for (int i = 0; i < count; i++) Post(tray, msg, wParam, lParam);
                return at;
            }
            Thread.Sleep(1);
        }
        return -1;
    }

    // Unix ms at which pid's window first came on screen, or -1 on timeout.
    public static long WaitOnScreen(int pid, int timeoutMs)
    {
        long deadline = Environment.TickCount64 + timeoutMs;
        while (Environment.TickCount64 < deadline)
        {
            if (!Parked(MainWindow(pid))) return Now();
            Thread.Sleep(2);
        }
        return -1;
    }

    // Unix ms at which pid's window was parked, or -1 on timeout.
    public static long WaitParked(int pid, int timeoutMs)
    {
        long deadline = Environment.TickCount64 + timeoutMs;
        while (Environment.TickCount64 < deadline)
        {
            IntPtr h = MainWindow(pid);
            if (h != IntPtr.Zero && Parked(h)) return Now();
            Thread.Sleep(2);
        }
        return -1;
    }

    public static bool StaysOnScreen(int pid, int durationMs)
    {
        long deadline = Environment.TickCount64 + durationMs;
        while (Environment.TickCount64 < deadline)
        {
            if (Parked(MainWindow(pid))) return false;
            Thread.Sleep(5);
        }
        return true;
    }

    public static bool StaysParked(int pid, int durationMs)
    {
        long deadline = Environment.TickCount64 + durationMs;
        while (Environment.TickCount64 < deadline)
        {
            if (!Parked(MainWindow(pid))) return false;
            Thread.Sleep(5);
        }
        return true;
    }

    // Every change of pid's window position and of whether it is the
    // foreground window during durationMs, one line each: "+ms left,top fg=...".
    public static string Trace(int pid, long startedUnixMs, int durationMs)
    {
        var lines = new StringBuilder();
        long deadline = Environment.TickCount64 + durationMs;
        string last = null;
        while (Environment.TickCount64 < deadline)
        {
            IntPtr h = MainWindow(pid);
            string now;
            if (h == IntPtr.Zero) now = "no window";
            else
            {
                var r = Rect(h);
                now = string.Format("{0},{1} fg={2}", r.Left, r.Top, GetForegroundWindow() == h);
            }
            if (now != last)
            {
                lines.AppendFormat("+{0} ms  {1}\n", Now() - startedUnixMs, now);
                last = now;
            }
            Thread.Sleep(5);
        }
        return lines.ToString();
    }

    // A thread that has just received a hotkey counts as having received the
    // last input, so it may pass the foreground on - the position Explorer is
    // in when the user double-clicks an exe. Uses a throwaway Ctrl+Alt+F24.
    public static bool GainForegroundRights()
    {
        const int id = 0xF0F;
        if (!RegisterHotKey(IntPtr.Zero, id, 3, 0x87)) return false;
        Press(3, 0x87);
        bool received = false;
        long deadline = Environment.TickCount64 + 1000;
        while (Environment.TickCount64 < deadline)
        {
            MSG m;
            if (PeekMessageW(out m, IntPtr.Zero, 0x0312, 0x0312, 1)) { received = true; break; }
            Thread.Sleep(5);
        }
        UnregisterHotKey(IntPtr.Zero, id);
        return received && AllowSetForegroundWindow(-1);
    }
}
