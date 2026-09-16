# Regression: a quick reopen must wait for the reset AND the final height.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
# Fresh data without deleting an existing test profile.
$WorkDir = Join-Path ([IO.Path]::GetTempPath()) ('copynote-testinstance-preparation-' + [guid]::NewGuid())
$p = Start-TestInstance
try {
  Check (Wait-PageReady) 'page ready'
  [void](Invoke-Page 'window.applyAutoHide(false)')
  $main = [CopyNoteProbe]::MainWindow($p.Id)
  $tray = [CopyNoteProbe]::Tray($TrayClass)
  if ($main -eq [IntPtr]::Zero -or [CopyNoteProbe]::WaitOnScreen($p.Id, 10000) -lt 0) {
    throw 'The initial native window did not appear'
  }

  foreach ($scenario in @('search', 'settings')) {
    # Hold the acknowledgement deliberately: correctness must not depend on
    # a fast renderer. Capture the DOM at the actual native show notification.
    [void](Invoke-Page "(() => {
      window.__originalPrepared ??= window.windowPrepared;
      window.windowPrepared = async (id, height) => {
        await new Promise(r => setTimeout(r, 600));
        return window.__originalPrepared(id, height);
      };
      window.__originalShown ??= window.__onShow;
      window.__showSnapshot = null;
      window.__onShow = () => {
        window.__showSnapshot = {query: document.getElementById('entry-search')?.value,
          main: !!document.querySelector('main[data-shell]')};
        return window.__originalShown();
      };
      const i = document.getElementById('entry-search');
      i.value = 'missing entry'; i.dispatchEvent(new Event('input', {bubbles:true}));
      if ('$scenario' === 'settings') window.__openSettings();
      return true;
    })()")
    Start-Sleep -Milliseconds 400
    [void][CopyNoteProbe]::ForceForeground($main)
    [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
    Start-Sleep -Milliseconds 50
    [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
    Start-Sleep -Milliseconds 300
    Check ([CopyNoteProbe]::Parked($main)) "$scenario stays parked until layout is acknowledged"
    $shown = [CopyNoteProbe]::WaitOnScreen($p.Id, 3000) -ge 0
    Check $shown "$scenario reopens after preparation"
    $heights = [Collections.Generic.HashSet[int]]::new()
    $deadline = (Get-Date).AddMilliseconds(400)
    while ((Get-Date) -lt $deadline) {
      $rect = [CopyNoteProbe]::Rect($main)
      [void]$heights.Add($rect.Bottom - $rect.Top)
      Start-Sleep -Milliseconds 5
    }
    Check ($shown -and $heights.Count -eq 1) "$scenario has constant height throughout slide-in"
    $snapshot = (Invoke-Page 'window.__showSnapshot') | ConvertFrom-Json
    Check ($snapshot.main -and $snapshot.query -eq '') "$scenario is already reset when shown"
  }
}
finally { Stop-TestInstance $p }
Complete-Checks
