# A window put away and asked back at once: Go waits for the page to get its
# next view ready (reset, measured) before it slides in, so the user never sees
# last session's search or a window still changing height - and it does not
# wait for ever, so a page that fails to answer cannot keep the window away.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

# Entries, so that a search with no match and the list it is reset to have
# different heights: on an empty list both are the same empty screen and the
# height check below could not fail.
$dataDir = Join-Path $WorkDir 'appdata\CopyNote'
New-Item -ItemType Directory -Force $dataDir | Out-Null
$entries = foreach ($n in 1..6) {
    @{ id = "seed-$n"; label = "Snippet $n"; value = "Some text to copy $n"; order = $n - 1
        createdAt = '2026-01-01T00:00:00Z'; updatedAt = '2026-01-01T00:00:00Z'
    }
}
@{ version = 2; entries = @($entries) } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $dataDir 'data.json') -Encoding utf8NoBOM

# How long Go waits for the page's answer: preparationTimeout in window_windows.go.
$GoWaitsMs = 500
# How long the answer is held back here. Long enough to tell from a page that
# answers at once; short enough that, with the 150 ms the page itself may take
# when no frames are being painted, the answer still beats Go's timeout.
$HeldMs = 200
$WM_CLOSE = 0x0010

# Replaces the page's answer to Go. 'held' delays it by $HeldMs, 'lost' never
# sends it, anything else restores the real binding.
function Set-Acknowledgement([string]$Mode) {
    [void](Invoke-Page "(() => {
      window.__realPrepared ??= window.windowPrepared;
      window.windowPrepared =
        '$Mode' === 'held' ? async (id, height) => { await new Promise((r) => setTimeout(r, $HeldMs)); return window.__realPrepared(id, height); }
        : '$Mode' === 'lost' ? async () => {}
        : window.__realPrepared;
      return true;
    })()")
}

# Puts the window away and asks it back through the hotkey 50 ms later, in the
# middle of the slide-out; returns how long it then stayed parked, in ms, or -1
# if it never went away or never came back. Both requests are posted messages:
# neither depends on which window is active, and nothing as slow as a DevTools
# call sits between them.
function Hide-ThenAskBack($Process, $Main, $Tray) {
    [void][CopyNoteProbe]::Post($Main, $WM_CLOSE, 0, 0)
    Start-Sleep -Milliseconds 50
    [void][CopyNoteProbe]::Post($Tray, $WM_HOTKEY, 1, 0)
    $parkedAt = [CopyNoteProbe]::WaitParked($Process.Id, 2000)
    if ($parkedAt -lt 0) { return -1 }
    $shownAt = [CopyNoteProbe]::WaitOnScreen($Process.Id, 5000)
    if ($shownAt -lt 0) { return -1 }
    return $shownAt - $parkedAt
}

$p = Start-TestInstance
try {
    Check (Wait-PageReady) 'the page loads'
    # A focus change outside the test must not put the window away mid-check.
    Start-Sleep -Milliseconds 300
    [void](Invoke-Page 'window.applyAutoHide(false)')
    $main = [CopyNoteProbe]::MainWindow($p.Id)
    $tray = [CopyNoteProbe]::Tray($TrayClass)
    if ($main -eq [IntPtr]::Zero -or [CopyNoteProbe]::WaitOnScreen($p.Id, 10000) -lt 0) {
        throw 'The initial native window did not appear'
    }
    Check ((Invoke-Page "document.querySelectorAll('[data-entry-id]').length") -eq '6') 'six entries are listed'

    # What the page looked like at the moment Go said the window was coming up.
    [void](Invoke-Page "(() => {
      const shown = window.__onShow;
      window.__onShow = (...args) => {
        window.__showSnapshot = { query: document.getElementById('entry-search')?.value, main: !!document.querySelector('main[data-shell]') };
        return shown(...args);
      };
      return true;
    })()")

    foreach ($scenario in 'search', 'settings') {
        Say "== Asked back right after hiding, from $scenario"
        Set-Acknowledgement 'held'
        [void](Invoke-Page "(() => {
          window.__showSnapshot = null;
          const i = document.getElementById('entry-search');
          i.value = 'missing entry'; i.dispatchEvent(new Event('input', { bubbles: true }));
          if ('$scenario' === 'settings') window.__openSettings();
          return true;
        })()")
        Start-Sleep -Milliseconds 500 # the page's own resize animation
        $away = Hide-ThenAskBack $p $main $tray
        $shown = $away -ge 0
        Check ($away -ge $HeldMs) 'it stays away while the page is getting ready' "$away ms"
        Check ($shown -and $away -lt $GoWaitsMs) 'and comes back on the page''s answer, before Go would have stopped waiting' "$away ms, Go waits $GoWaitsMs"
        $heights = [Collections.Generic.HashSet[int]]::new()
        $deadline = (Get-Date).AddMilliseconds(400)
        while ((Get-Date) -lt $deadline) {
            $rect = [CopyNoteProbe]::Rect($main)
            [void]$heights.Add($rect.Bottom - $rect.Top)
            Start-Sleep -Milliseconds 5
        }
        Check ($shown -and $heights.Count -eq 1) 'at one height from the first frame of the slide to the last' ("heights seen: " + (($heights | Sort-Object) -join ', '))
        $snapshot = (Invoke-Page 'window.__showSnapshot') | ConvertFrom-Json
        Check ($snapshot.main -and $snapshot.query -eq '') 'already showing the list with an empty search' ("main view $($snapshot.main), query '$($snapshot.query)'")
    }

    Say '== The page never answers'
    Set-Acknowledgement 'lost'
    $away = Hide-ThenAskBack $p $main $tray
    Check (($away -ge $GoWaitsMs - 100) -and ($away -lt $GoWaitsMs + 1000)) 'the window comes back all the same, once Go stops waiting' "$away ms, Go waits $GoWaitsMs"
    Start-Sleep -Milliseconds 800
    # Hidden with nobody asking for it back: still no answer, and the next
    # show - much later - must not find the window stuck.
    [void](Invoke-Page 'window.hide()')
    Check ([CopyNoteProbe]::WaitParked($p.Id, 2000) -ge 0) 'it can be put away again' ([CopyNoteProbe]::Where($main))
    Start-Sleep -Milliseconds 1200
    [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
    Check ([CopyNoteProbe]::WaitOnScreen($p.Id, 3000) -ge 0) 'and opened again later' ([CopyNoteProbe]::Where($main))
    Set-Acknowledgement 'real'
}
finally { Stop-TestInstance $p }
Complete-Checks
