# What a click on the tray icon and the global hotkey do with a window that is
# already on screen: put it away when it was the one in use, bring it forward
# when another program was. By the time a click arrives the shell has taken the
# foreground to deliver it, so the app cannot ask which window is active - it
# goes by the deactivation Windows reported to it, and only a real change of
# foreground shows whether it reads that right. Also here, because it needs a
# page that really has focus: where focus lands when a dialog opened with the
# mouse closes.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

if (-not $Interactive) {
    Skip 'what the tray icon and the hotkey do with a window that is on screen' $NotInteractive
    Complete-Checks
}

# A click on the icon and a press of the hotkey, as the messages they end up as.
# A real one comes with the right to take the foreground - the shell grants it
# for a click, Windows for a hotkey - so that right is obtained first.
function Send-TrayClick($Tray) {
    [void][CopyNoteProbe]::GainForegroundRights()
    [void][CopyNoteProbe]::Post($Tray, $TRAY_CALLBACK, 0, $WM_LBUTTONUP)
}
function Send-Hotkey($Tray) {
    [void][CopyNoteProbe]::GainForegroundRights()
    [void][CopyNoteProbe]::Post($Tray, $WM_HOTKEY, 1, 0)
}

# Hands the foreground to a window and gives the app a moment to hear of it.
function Switch-To([IntPtr]$Hwnd) {
    $done = [CopyNoteProbe]::ForceForeground($Hwnd)
    Start-Sleep -Milliseconds 300
    return $done
}

function Show-Window($Process, $Tray, $Main) {
    if ([CopyNoteProbe]::Parked($Main)) {
        Send-TrayClick $Tray
        [void][CopyNoteProbe]::WaitOnScreen($Process.Id, 3000)
    }
    Start-Sleep -Milliseconds 600 # the slide, and the guard against hiding right after a show
    return (Set-WindowForeground $Main)
}

# "Another program" is a window of this script's own: the checks then do not
# depend on what the user happens to have in front, and leave it alone.
$taskbar = [CopyNoteProbe]::Taskbar()
$other = [CopyNoteProbe]::OtherProgramWindow()
if ($taskbar -eq [IntPtr]::Zero -or $other -eq [IntPtr]::Zero) { throw 'No taskbar, or no window could be created to stand in for another program.' }
[void][CopyNoteProbe]::GainForegroundRights()
$p = Start-TestInstance
try {
    Check (Wait-PageReady) 'the page loads'
    [void](Invoke-Page "window.create('Alpha', 'value')")
    [void](Invoke-Page 'location.reload()')
    Start-Sleep -Milliseconds 300
    [void](Wait-PageReady)
    $main = [CopyNoteProbe]::MainWindow($p.Id)
    $tray = [CopyNoteProbe]::Tray($TrayClass)
    Check ([CopyNoteProbe]::WaitOnScreen($p.Id, 10000) -ge 0) 'the window is on screen' ([CopyNoteProbe]::Where($main))
    # Loading the settings applies auto-hide; pin the window after that.
    Start-Sleep -Milliseconds 500
    [void](Invoke-Page 'window.applyAutoHide(false)')

    Say '== Pinned and in use: reaching for the tray icon, then the click'
    Check (Show-Window $p $tray $main) 'the window is the active one'
    Check (Switch-To $taskbar) 'the taskbar takes the foreground, as it does when the pointer goes for the icon'
    Check (-not [CopyNoteProbe]::Parked($main)) 'the pinned window stays up meanwhile'
    Send-TrayClick $tray
    Check ([CopyNoteProbe]::WaitParked($p.Id, 3000) -ge 0) 'the click puts it away' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds 500

    Say '== Pinned while another program is in use: the click'
    Check (Show-Window $p $tray $main) 'the window is back and active'
    Check (Switch-To $other) 'the other program takes the foreground'
    Check (-not [CopyNoteProbe]::Parked($main)) 'the pinned window stays up while it is used'
    [void](Switch-To $taskbar)
    Send-TrayClick $tray
    Start-Sleep -Milliseconds 600
    Check (-not [CopyNoteProbe]::Parked($main)) 'the click does not close a window that was not in use' ([CopyNoteProbe]::Where($main))
    Check ([CopyNoteProbe]::Foreground() -eq $main) 'it brings it forward'
    Send-TrayClick $tray
    # No shell in between this time: the posted click finds the window active.
    Check ([CopyNoteProbe]::WaitParked($p.Id, 3000) -ge 0) 'and a second click, now that it is in use, puts it away' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds 500

    Say '== Pinned while another program is in use: the hotkey'
    Check (Show-Window $p $tray $main) 'the window is back and active'
    Check (Switch-To $other) 'the other program takes the foreground'
    Send-Hotkey $tray
    Start-Sleep -Milliseconds 600
    Check ((-not [CopyNoteProbe]::Parked($main)) -and ([CopyNoteProbe]::Foreground() -eq $main)) 'the hotkey brings the window forward' ([CopyNoteProbe]::Where($main))
    Send-Hotkey $tray
    Check ([CopyNoteProbe]::WaitParked($p.Id, 3000) -ge 0) 'and pressed again puts it away' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds 500

    Say '== Not pinned: the click that hid the window does not bring it back'
    [void](Invoke-Page 'window.applyAutoHide(true)')
    Check (Show-Window $p $tray $main) 'the window is back and active'
    # Reaching for the icon hides the window; the click itself arrives a
    # moment later and must not undo that.
    [void][CopyNoteProbe]::ForceForeground($taskbar)
    Start-Sleep -Milliseconds 20
    [void][CopyNoteProbe]::Post($tray, $TRAY_CALLBACK, 0, $WM_LBUTTONUP)
    $hidden = [CopyNoteProbe]::WaitParked($p.Id, 3000) -ge 0
    Start-Sleep -Milliseconds 700
    Check ($hidden -and [CopyNoteProbe]::Parked($main)) 'it goes away and stays away' ([CopyNoteProbe]::Where($main))
    Send-TrayClick $tray
    Check ([CopyNoteProbe]::WaitOnScreen($p.Id, 3000) -ge 0) 'a click after that opens it' ([CopyNoteProbe]::Where($main))

    Say '== A dialog opened with the mouse hands focus back to the card'
    [void](Invoke-Page 'window.applyAutoHide(false)')
    Start-Sleep -Milliseconds 600
    [void](Set-WindowForeground $main)
    $edit = "document.querySelector('[data-entry-id] button[aria-keyshortcuts=F2]')"
    $pt = Invoke-Page "(() => { const r = $edit.getBoundingClientRect(); return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) } })()" | ConvertFrom-Json
    [void](Send-PageCommand 'Input.dispatchMouseEvent' @{ type = 'mouseMoved'; x = $pt.x; y = $pt.y })
    [void](Send-PageCommand 'Input.dispatchMouseEvent' @{ type = 'mousePressed'; x = $pt.x; y = $pt.y; button = 'left'; buttons = 1; clickCount = 1 })
    [void](Send-PageCommand 'Input.dispatchMouseEvent' @{ type = 'mouseReleased'; x = $pt.x; y = $pt.y; button = 'left'; buttons = 0; clickCount = 1 })
    $opened = $false
    $deadline = (Get-Date).AddSeconds(3)
    while (-not $opened -and (Get-Date) -lt $deadline) {
        $opened = (Invoke-Page "!!document.querySelector('[role=dialog] input')") -eq 'true'
        if (-not $opened) { Start-Sleep -Milliseconds 100 }
    }
    Check $opened 'a click on the pencil opens the edit form'
    # The pointer leaves the card, and with it go the edit and delete buttons.
    [void](Send-PageCommand 'Input.dispatchMouseEvent' @{ type = 'mouseMoved'; x = 10; y = 10 })
    [void](Send-PageCommand 'Input.dispatchKeyEvent' @{ type = 'rawKeyDown'; key = 'Escape'; code = 'Escape'; windowsVirtualKeyCode = 27; nativeVirtualKeyCode = 27 })
    [void](Send-PageCommand 'Input.dispatchKeyEvent' @{ type = 'keyUp'; key = 'Escape'; code = 'Escape'; windowsVirtualKeyCode = 27; nativeVirtualKeyCode = 27 })
    $closed = $false
    $deadline = (Get-Date).AddSeconds(3)
    while (-not $closed -and (Get-Date) -lt $deadline) {
        $closed = (Invoke-Page "!document.querySelector('[role=dialog]')") -eq 'true'
        if (-not $closed) { Start-Sleep -Milliseconds 100 }
    }
    Start-Sleep -Milliseconds 200
    $focus = Invoke-Page "(() => { const a = document.activeElement; return { card: !!a && a.hasAttribute('data-card-focus'), where: a ? (a.getAttribute('aria-label') || a.id || a.tagName) : 'nothing' } })()" | ConvertFrom-Json
    Check ($closed -and $focus.card) 'Escape closes it and focus is on the card, not on a button nobody can see' "closed $closed, focus on '$($focus.where)'"
    Check (-not [CopyNoteProbe]::Parked($main)) 'the window is still up'
}
finally {
    Stop-TestInstance $p
    [CopyNoteProbe]::Destroy($other)
}

Complete-Checks
