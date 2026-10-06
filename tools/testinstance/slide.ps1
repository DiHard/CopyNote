# Slides and resizes in the real window. The page reports the height it needs
# on every frame of its own resize animation, while Go slides the window in and
# out on a goroutine of its own; only a running exe shows how the two meet. The
# hide checks here once left the window on screen with Go taking it for hidden:
# Escape, the close button and auto-hide then did nothing until the tray icon
# was clicked.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

$VK_DOWN = 0x28
$VK_RETURN = 0x0D
$WM_KEYDOWN = 0x0100
$WM_RBUTTONUP = 0x0205
# Long enough for the page's resize animation (about 15 frames) and a slide.
$SettleMs = 800

$Search = "document.getElementById('entry-search')"
# A synthetic keydown on the search box goes through its own handler and then
# App's window handler: the cascade a real Escape takes.
$PressEscape = "$Search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))"

# Types into the search box the way a keystroke does: bind:value listens to input.
function Set-Query([string]$Text) {
    [void](Invoke-Page "(() => { const i = $Search; i.value = '$Text'; i.dispatchEvent(new Event('input', { bubbles: true })); return true })()")
}

function Get-Query { return (Invoke-Page "$Search.value") | ConvertFrom-Json }

# Presses and releases X as trusted input: the page cannot tell it from the keyboard.
function Send-Letter {
    [void](Send-PageCommand 'Input.dispatchKeyEvent' @{ type = 'keyDown'; key = 'x'; code = 'KeyX'; text = 'x'; windowsVirtualKeyCode = 88; nativeVirtualKeyCode = 88 })
    [void](Send-PageCommand 'Input.dispatchKeyEvent' @{ type = 'keyUp'; key = 'x'; code = 'KeyX'; windowsVirtualKeyCode = 88; nativeVirtualKeyCode = 88 })
}

function Get-Height($Rect) { return $Rect.Bottom - $Rect.Top }

function Show-FromTray([int]$ProcessId) {
    [void][CopyNoteProbe]::Post([CopyNoteProbe]::Tray($TrayClass), $TRAY_CALLBACK, 0, $WM_LBUTTONUP)
    [void][CopyNoteProbe]::WaitOnScreen($ProcessId, 3000)
    Start-Sleep -Milliseconds $SettleMs
}

[void][CopyNoteProbe]::GainForegroundRights()
$p = Start-TestInstance
try {
    Check (Wait-PageReady) 'the page loads'
    foreach ($n in 1..8) { [void](Invoke-Page "window.create('Snippet $n', 'Some text to copy $n')") }
    [void](Invoke-Page 'location.reload()')
    Start-Sleep -Milliseconds 300
    [void](Wait-PageReady)
    $listed = $false
    $deadline = (Get-Date).AddSeconds(10)
    while (-not $listed -and (Get-Date) -lt $deadline) {
        $listed = (Invoke-Page "document.querySelectorAll('[data-entry-id]').length === 8") -eq 'true'
        if (-not $listed) { Start-Sleep -Milliseconds 200 }
    }
    Check $listed 'eight entries are listed'
    # Loading the settings applies auto-hide; a focus change outside the test must
    # not put the window away in the middle of a check.
    Start-Sleep -Milliseconds 300
    [void](Invoke-Page 'window.applyAutoHide(false)')
    $main = [CopyNoteProbe]::MainWindow($p.Id)
    # Seeding/reloading the page can outlast the startup focus guard. Restore
    # a window that auto-hid during setup before measuring the tray corner.
    if ([CopyNoteProbe]::Parked($main)) { Show-FromTray $p.Id }
    Check ([CopyNoteProbe]::WaitOnScreen($p.Id, 10000) -ge 0) 'the window is on screen' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds $SettleMs
    $full = [CopyNoteProbe]::Rect($main)
    $corner = $full.Bottom

    Say '== A search, then Escape, on screen'
    Set-Query 'snippet 5'
    Start-Sleep -Milliseconds $SettleMs
    $filtered = [CopyNoteProbe]::Rect($main)
    [void](Invoke-Page $PressEscape)
    Start-Sleep -Milliseconds $SettleMs
    $cleared = [CopyNoteProbe]::Rect($main)
    Check ((Get-Height $filtered) -lt (Get-Height $full) - 100) 'a search that leaves one entry makes the window shorter' ('{0} -> {1} px' -f (Get-Height $full), (Get-Height $filtered))
    Check ([math]::Abs((Get-Height $cleared) - (Get-Height $full)) -le 2) 'Escape brings the full height back' ('{0} px' -f (Get-Height $cleared))
    Check (([math]::Abs($filtered.Bottom - $corner) -le 2) -and ([math]::Abs($cleared.Bottom - $corner) -le 2)) 'the bottom edge stays in the corner' ('bottom {0}, then {1}; corner {2}' -f $filtered.Bottom, $cleared.Bottom, $corner)

    Say '== Escape twice, quickly: hidden while the window is still growing back'
    Set-Query 'snippet 5'
    Start-Sleep -Milliseconds $SettleMs
    [void](Invoke-Page "(async () => { $PressEscape; await new Promise((r) => setTimeout(r, 60)); $PressEscape; return true })()")
    $parked = [CopyNoteProbe]::WaitParked($p.Id, 2000) -ge 0
    Check $parked 'the second Escape puts the window away' ([CopyNoteProbe]::Where($main))
    if (-not $parked) {
        [void](Invoke-Page $PressEscape)
        Start-Sleep -Milliseconds 500
        Say "       one more Escape leaves it $([CopyNoteProbe]::Where($main))"
    }
    Show-FromTray $p.Id

    Say '== Put away right after typing, as Enter does after a copy'
    Check (-not [CopyNoteProbe]::Parked($main)) 'the window is back on screen' ([CopyNoteProbe]::Where($main))
    [void](Invoke-Page "(async () => { const i = $Search; i.value = 'snippet 5'; i.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 60)); window.hide(); return true })()")
    Check ([CopyNoteProbe]::WaitParked($p.Id, 2000) -ge 0) 'it is parked, though the page was still shrinking it' ([CopyNoteProbe]::Where($main))

    Say '== Reopened after that search (the trace is for reading, not a check)'
    $clicked = [CopyNoteProbe]::Now()
    [void][CopyNoteProbe]::Post([CopyNoteProbe]::Tray($TrayClass), $TRAY_CALLBACK, 0, $WM_LBUTTONUP)
    $trace = [CopyNoteProbe]::Trace($p.Id, $clicked, 1000)
    foreach ($line in ($trace -split "`n")) { if ($line) { Say "       $line" } }
    $reopened = [CopyNoteProbe]::Rect($main)
    $query = Get-Query
    Check (($query -eq '') -and ([math]::Abs((Get-Height $reopened) - (Get-Height $full)) -le 2)) 'the search is cleared and the window is back at full height' ('query "{0}", {1} px' -f $query, (Get-Height $reopened))
    Check ([math]::Abs($reopened.Bottom - $corner) -le 2) 'in the corner' ('bottom {0}, corner {1}' -f $reopened.Bottom, $corner)

    Say '== Asked back while it is still sliding away'
    $tray = [CopyNoteProbe]::Tray($TrayClass)
    # The hide comes from the page and the show from the hotkey, so neither
    # depends on which window is active. The window finishes going away, the
    # page gets its next view ready, and only then does it come back.
    [void](Invoke-Page 'window.hide()')
    Start-Sleep -Milliseconds 50
    [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
    $back = [CopyNoteProbe]::WaitOnScreen($p.Id, 3000) -ge 0
    Start-Sleep -Milliseconds $SettleMs
    Check ($back -and -not [CopyNoteProbe]::Parked($main)) 'the window ends up on screen' ([CopyNoteProbe]::Where($main))

    Say '== The hotkey twice in quick succession'
    # The first press only puts the window away if it is the active one;
    # brought forward instead, it would end up on screen without having moved.
    if ($Interactive -and [CopyNoteProbe]::ForceForeground($main) -and ([CopyNoteProbe]::Foreground() -eq $main)) {
        [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
        $leaving = [CopyNoteProbe]::Rect($main).Top
        Start-Sleep -Milliseconds 50
        $moved = [CopyNoteProbe]::Rect($main).Top -ne $leaving
        [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
        Start-Sleep -Milliseconds $SettleMs
        Check ($moved -and -not [CopyNoteProbe]::Parked($main)) 'it goes away and comes back' ("was sliding $moved, now " + [CopyNoteProbe]::Where($main))
    }
    else {
        $why = if ($Interactive) { 'the window could not be made the active one' } else { $NotInteractive }
        Skip 'the hotkey puts the active window away and a second press brings it back' $why
    }

    Say '== A parked window still has the keyboard until the user clicks elsewhere'
    # A trusted keystroke, as one typed for another program would arrive. With
    # the window on screen it has to land in the search box: only then does
    # its absence from a parked window say anything.
    [void](Invoke-Page "$Search.focus()")
    Send-Letter
    $reaches = (Get-Query) -eq 'x'
    Set-Query ''
    Start-Sleep -Milliseconds $SettleMs
    [void](Invoke-Page 'window.hide()')
    Check ([CopyNoteProbe]::WaitParked($p.Id, 2000) -ge 0) 'the window can still be put away' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds $SettleMs
    Send-Letter
    $typed = Get-Query
    if ($reaches) { Check ($typed -eq '') 'what is typed meanwhile does not land in the search box' ('query "{0}"' -f $typed) }
    else { Skip 'what is typed meanwhile does not land in the search box' 'a key sent through DevTools did not reach the search box even with the window on screen' }
    # And if something does get in, it is not there when the window opens.
    Set-Query 'left behind'
    Show-FromTray $p.Id
    $query = Get-Query
    Check ($query -eq '') 'text that reached the parked window is gone when it opens' ('query "{0}"' -f $query)
    [void](Invoke-Page 'window.hide()')
    Check ([CopyNoteProbe]::WaitParked($p.Id, 2000) -ge 0) 'the window is away again' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds $SettleMs

    Say '== Settings from the tray menu while the window is away'
    [void][CopyNoteProbe]::GainForegroundRights()
    [void][CopyNoteProbe]::Post($tray, $TRAY_CALLBACK, 0, $WM_RBUTTONUP)
    $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
    Check ($menu -ne [IntPtr]::Zero) "the tray icon's menu opens"
    if ($menu -ne [IntPtr]::Zero) {
        foreach ($vk in @($VK_DOWN, $VK_DOWN, $VK_RETURN)) { [void][CopyNoteProbe]::Post($menu, $WM_KEYDOWN, $vk, 0) }
        $shown = [CopyNoteProbe]::WaitOnScreen($p.Id, 3000) -ge 0
        Start-Sleep -Milliseconds $SettleMs
        $settings = (Invoke-Page "!!document.querySelector('h1')") -eq 'true'
        $r = [CopyNoteProbe]::Rect($main)
        Check ($shown -and $settings) 'Settings comes up' "shown $shown, settings $settings"
        Check ([math]::Abs($r.Bottom - $corner) -le 2) 'and ends in the corner, resized during its slide' ('{0} -> {1} px, bottom {2}, corner {3}' -f (Get-Height $full), (Get-Height $r), $r.Bottom, $corner)
    }
}
finally { Stop-TestInstance $p }

Complete-Checks
