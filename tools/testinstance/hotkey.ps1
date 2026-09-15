# The global shortcut, typed for real: it toggles the window and reopening
# clears the search; switching the combination registers the new one and
# releases the old; Windows refusing one leaves the previous one working.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

$ctrlAlt = $MOD_CONTROL -bor $MOD_ALT
foreach ($vk in $VK_N, $VK_M) {
  if (Test-HotkeyHeld $ctrlAlt $vk) {
    throw ('Ctrl+Alt+{0} is held by another program - a daily CopyNote that has the hotkey? Quit it and run again.' -f [char]$vk)
  }
}

# Types Ctrl+Alt+<Vk> and returns what became of the window: 'shown' or
# 'hidden'. Returns 'unregistered' without typing when nobody holds the
# combination - the keys would land in whatever window has focus.
function Send-Combination([uint32]$Vk) {
  if (-not (Test-HotkeyHeld $ctrlAlt $Vk)) { return 'unregistered' }
  $wasParked = [CopyNoteProbe]::Parked([CopyNoteProbe]::MainWindow($p.Id))
  [CopyNoteProbe]::Press($ctrlAlt, [byte]$Vk)
  if ($wasParked) { [void][CopyNoteProbe]::WaitOnScreen($p.Id, 3000) }
  else { [void][CopyNoteProbe]::WaitParked($p.Id, 3000) }
  Start-Sleep -Milliseconds 400 # the slide
  if ([CopyNoteProbe]::Parked([CopyNoteProbe]::MainWindow($p.Id))) { return 'hidden' }
  return 'shown'
}

function Set-Hotkey([string]$Spec) {
  return Invoke-Page "window.applyHotkey('$Spec').then(() => 'accepted', (e) => 'refused: ' + e)"
}

# Parked at first, so the first keystroke has a known effect.
$p = Start-TestInstance @('--autostart')
try {
  Check (Wait-PageReady) 'the page loads'
  Start-Sleep -Milliseconds 500 # the UI re-applies the stored hotkey on load
  Check (Test-HotkeyHeld $ctrlAlt $VK_N) 'the test instance registers the default Ctrl+Alt+N'

  [void](Invoke-Page "(() => { const i = document.getElementById('entry-search'); i.value = 'abc'; i.dispatchEvent(new Event('input', { bubbles: true })); return i.value })()")
  $result = Send-Combination $VK_N
  Check ($result -eq 'shown') 'Ctrl+Alt+N brings the window up' $result
  Check ([CopyNoteProbe]::Foreground() -eq [CopyNoteProbe]::MainWindow($p.Id)) 'the window takes the foreground'
  $page = Invoke-Page "({ query: document.getElementById('entry-search').value, focused: document.activeElement && document.activeElement.id })" | ConvertFrom-Json
  Check (($page.query -eq '') -and ($page.focused -eq 'entry-search')) 'the search typed before is cleared and has focus' ("query '{0}', focus on '{1}'" -f $page.query, $page.focused)
  $result = Send-Combination $VK_N
  Check ($result -eq 'hidden') 'Ctrl+Alt+N again puts it away' $result

  $answer = Set-Hotkey 'Win+V'
  Check ($answer -like '*refused*') 'Windows refuses Win+V, which it keeps for itself' $answer
  Check (Test-HotkeyHeld $ctrlAlt $VK_N) 'Ctrl+Alt+N still works after the refusal'

  $answer = Set-Hotkey 'Ctrl+Alt+M'
  Check ($answer -eq '"accepted"') 'switching to Ctrl+Alt+M is accepted' $answer
  Check ((Test-HotkeyHeld $ctrlAlt $VK_M) -and -not (Test-HotkeyHeld $ctrlAlt $VK_N)) 'Ctrl+Alt+M is registered and Ctrl+Alt+N released'
  $result = Send-Combination $VK_M
  Check ($result -eq 'shown') 'Ctrl+Alt+M brings the window up' $result
  $result = Send-Combination $VK_M
  Check ($result -eq 'hidden') 'and puts it away' $result

  $answer = Set-Hotkey 'off'
  Check (($answer -eq '"accepted"') -and -not (Test-HotkeyHeld $ctrlAlt $VK_M)) "'off' releases the combination" $answer

  $answer = Set-Hotkey ''
  Check (($answer -eq '"accepted"') -and (Test-HotkeyHeld $ctrlAlt $VK_N)) 'an empty setting brings back Ctrl+Alt+N' $answer
}
finally { Stop-TestInstance $p }

Check (-not (Test-HotkeyHeld $ctrlAlt $VK_N)) 'quitting releases Ctrl+Alt+N'
Complete-Checks
