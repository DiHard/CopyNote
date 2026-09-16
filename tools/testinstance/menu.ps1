# The entry context menu in the real window. Go draws it with the tray's own
# popup (internal/popupmenu), so only a running exe shows whether it opens where
# it should, leaves the window up, takes the keyboard, hands activation and
# focus back, and closes with the window. The tray icon's menu shares that code
# now, so it is checked too.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

$VK_DOWN = 0x28
$VK_RETURN = 0x0D
$VK_ESCAPE = 0x1B
$WM_KEYDOWN = 0x0100
$WM_RBUTTONUP = 0x0205

# Keys go to the menu window as messages: nothing is typed anywhere else.
function Send-MenuKeys([IntPtr]$Menu, [int[]]$Keys) {
  foreach ($vk in $Keys) { [void][CopyNoteProbe]::Post($Menu, $WM_KEYDOWN, $vk, 0) }
}

# Clicks item $Index with a posted WM_LBUTTONUP in the middle of its row. Rows
# follow the menu's layout in internal/popupmenu: 6 px padding, 36 px rows and
# a 9 px separator after the third item, all at 96 DPI.
function Invoke-MenuItemClick([IntPtr]$Menu, [int]$Index) {
  $tops = @(6, 42, 78, 114, 123, 159)
  $scale = [CopyNoteProbe]::Dpi($Menu) / 96.0
  $x = [int][math]::Round(30 * $scale)
  $y = [int][math]::Round(($tops[$Index] + 18) * $scale)
  [void][CopyNoteProbe]::Post($Menu, $WM_LBUTTONUP, 0, ($y -shl 16) -bor $x)
}

# Right-clicks the middle of the card at $Index with trusted DevTools input and
# returns the point, in CSS pixels.
function Invoke-RightClick([int]$Index) {
  $pt = Invoke-Page "(() => { const r = document.querySelectorAll('[data-entry-id]')[$Index].getBoundingClientRect(); return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) } })()" | ConvertFrom-Json
  [void](Send-PageCommand 'Input.dispatchMouseEvent' @{ type = 'mousePressed'; x = $pt.x; y = $pt.y; button = 'right'; buttons = 2; clickCount = 1 })
  [void](Send-PageCommand 'Input.dispatchMouseEvent' @{ type = 'mouseReleased'; x = $pt.x; y = $pt.y; button = 'right'; buttons = 0; clickCount = 1 })
  return $pt
}

# The foreground a click into the window would give it. Windows does not count
# DevTools input as input, so without this the window never had activation for
# its menu to take.
function Set-WindowForeground([IntPtr]$Hwnd) {
  [void][CopyNoteProbe]::ForceForeground($Hwnd)
  Start-Sleep -Milliseconds 150
  return ([CopyNoteProbe]::Foreground() -eq $Hwnd)
}

function Get-Order { return (Invoke-Page "window.list().then((l) => l.map((e) => e.label).join(','))") | ConvertFrom-Json }

# Whether a CSS y inside the window lands on the menu's top or bottom edge:
# the menu opens below that point, or above it near the bottom of the screen.
function Test-MenuEdgeAt([IntPtr]$Menu, [IntPtr]$Main, [double]$CssY) {
  $scale = [CopyNoteProbe]::Dpi($Main) / 96.0
  $y = [CopyNoteProbe]::Rect($Main).Top + [math]::Round($CssY * $scale)
  $r = [CopyNoteProbe]::Rect($Menu)
  $ok = ([math]::Abs($r.Top - $y) -le 2) -or ([math]::Abs($r.Bottom - $y) -le 2)
  return @{ Ok = $ok; Detail = "point at y=$y, menu from $($r.Top) to $($r.Bottom)" }
}

[void][CopyNoteProbe]::GainForegroundRights()
$p = Start-TestInstance
try {
  Check (Wait-PageReady) 'the page loads'
  foreach ($label in 'Gamma', 'Beta', 'Alpha') { [void](Invoke-Page "window.create('$label', 'value')") }
  [void](Invoke-Page 'location.reload()')
  Start-Sleep -Milliseconds 300
  [void](Wait-PageReady)
  $listed = $false
  $deadline = (Get-Date).AddSeconds(10)
  while (-not $listed -and (Get-Date) -lt $deadline) {
    $listed = (Invoke-Page "document.querySelectorAll('[data-entry-id]').length === 3") -eq 'true'
    if (-not $listed) { Start-Sleep -Milliseconds 200 }
  }
  Check $listed 'three entries are listed' (Get-Order)
  $main = [CopyNoteProbe]::MainWindow($p.Id)
  Check ([CopyNoteProbe]::WaitOnScreen($p.Id, 10000) -ge 0) 'the window is on screen' ([CopyNoteProbe]::Where($main))
  Start-Sleep -Milliseconds 500
  Check (Set-WindowForeground $main) 'the window has the foreground, as after a click into it'

  # A right click, then the keyboard inside the menu.
  $click = Invoke-RightClick 0
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  Check ($menu -ne [IntPtr]::Zero) 'a right click on a card opens the menu'
  if ($menu -ne [IntPtr]::Zero) {
    Check ([CopyNoteProbe]::Owner($menu) -eq $main) 'the main window owns it'
    $edge = Test-MenuEdgeAt $menu $main $click.y
    Check $edge.Ok 'it opens at the pointer' $edge.Detail
    $stays = [CopyNoteProbe]::StaysOnScreen($p.Id, 800)
    Check ($stays -and [CopyNoteProbe]::Menu($p.Id) -ne [IntPtr]::Zero) 'the window stays up while the menu is open'
    Check ([CopyNoteProbe]::Foreground() -eq $menu) 'the menu takes activation from its window'

    Send-MenuKeys $menu @($VK_DOWN, $VK_DOWN, $VK_DOWN, $VK_DOWN, $VK_RETURN)
    Check ([CopyNoteProbe]::WaitMenuGone($p.Id, 2000)) 'Down four times and Enter close it'
    Start-Sleep -Milliseconds 400
    $order = Get-Order
    Check ($order -eq 'Beta,Alpha,Gamma') 'and pick "Move down", skipping the separator and the disabled "Move up"' $order
    Check (-not [CopyNoteProbe]::Parked($main)) 'the window is still up after the pick'
    Check ([CopyNoteProbe]::Foreground() -eq $main) 'activation returns to the window'
    Check ((Invoke-Page 'document.hasFocus()') -eq 'true') 'and keyboard focus to the page'
  }

  # Escape.
  [void](Set-WindowForeground $main)
  [void](Invoke-RightClick 0)
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  Send-MenuKeys $menu @($VK_ESCAPE)
  $gone = [CopyNoteProbe]::WaitMenuGone($p.Id, 2000)
  Start-Sleep -Milliseconds 400
  $dialog = (Invoke-Page "!!document.querySelector('[role=dialog]')") -eq 'true'
  $order = Get-Order
  Check (($menu -ne [IntPtr]::Zero) -and $gone -and -not $dialog -and ($order -eq 'Beta,Alpha,Gamma')) 'Escape closes it and changes nothing' "closed $gone, dialog $dialog, order $order"

  # A mouse click on an item.
  [void](Set-WindowForeground $main)
  [void](Invoke-RightClick 0)
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  if ($menu -ne [IntPtr]::Zero) { Invoke-MenuItemClick $menu 2 }
  $gone = [CopyNoteProbe]::WaitMenuGone($p.Id, 2000)
  Start-Sleep -Milliseconds 600 # the dialog waits for the window to grow
  $confirm = (Invoke-Page "!!document.querySelector('[role=dialog] [data-initial-focus]')") -eq 'true'
  Check (($menu -ne [IntPtr]::Zero) -and $gone -and $confirm) 'a click on "Delete" asks to confirm the delete' "closed $gone, confirmation $confirm"
  [void](Invoke-Page "document.querySelector('[role=dialog] [data-initial-focus]')?.click()")
  Start-Sleep -Milliseconds 400

  # The menu key on the focused card.
  [void](Set-WindowForeground $main)
  [void](Invoke-Page "document.querySelector('[data-card-focus]').focus()")
  [void](Send-PageCommand 'Input.dispatchKeyEvent' @{ type = 'rawKeyDown'; key = 'ContextMenu'; code = 'ContextMenu'; windowsVirtualKeyCode = 93; nativeVirtualKeyCode = 93 })
  [void](Send-PageCommand 'Input.dispatchKeyEvent' @{ type = 'keyUp'; key = 'ContextMenu'; code = 'ContextMenu'; windowsVirtualKeyCode = 93; nativeVirtualKeyCode = 93 })
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  Check ($menu -ne [IntPtr]::Zero) 'the menu key opens it on the focused card'
  if ($menu -ne [IntPtr]::Zero) {
    $bottom = [double](Invoke-Page "document.querySelector('[data-entry-id]').getBoundingClientRect().bottom")
    $edge = Test-MenuEdgeAt $menu $main $bottom
    Check $edge.Ok 'under the card rather than at the pointer' $edge.Detail
    Send-MenuKeys $menu @($VK_DOWN, $VK_RETURN)
    [void][CopyNoteProbe]::WaitMenuGone($p.Id, 2000)
    Start-Sleep -Milliseconds 600 # the modal waits for the window to grow
    $edited = Invoke-Page "(document.querySelector('[role=dialog] input') || {}).value || null"
    Check ($edited -eq '"Beta"') 'with its first item highlighted: Down and Enter pick "Edit"' $edited
    [void](Invoke-Page "[...document.querySelectorAll('[role=dialog] button')].find((b) => b.type === 'button')?.click()")
    Start-Sleep -Milliseconds 400
  }

  # Another window takes the foreground while the menu is open.
  [void](Set-WindowForeground $main)
  [void](Invoke-RightClick 0)
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  $taskbar = [CopyNoteProbe]::ForceForeground([CopyNoteProbe]::Taskbar())
  $gone = [CopyNoteProbe]::WaitMenuGone($p.Id, 2000)
  $parked = [CopyNoteProbe]::WaitParked($p.Id, 3000) -ge 0
  Check (($menu -ne [IntPtr]::Zero) -and $gone -and $parked) 'switching to another window closes the menu, and auto-hide puts the window away' "taskbar active $taskbar, closed $gone, parked $parked"

  # The window is put away while its menu is open.
  $tray = [CopyNoteProbe]::Tray($TrayClass)
  if ([CopyNoteProbe]::Parked($main)) {
    [void][CopyNoteProbe]::GainForegroundRights()
    [void][CopyNoteProbe]::Post($tray, $TRAY_CALLBACK, 0, $WM_LBUTTONUP)
    [void][CopyNoteProbe]::WaitOnScreen($p.Id, 3000)
    Start-Sleep -Milliseconds 500
  }
  Check (-not [CopyNoteProbe]::Parked($main)) 'the window is on screen again'
  [void](Set-WindowForeground $main)
  [void](Invoke-RightClick 0)
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  [void][CopyNoteProbe]::Post($tray, $WM_HOTKEY, 1, 0)
  $gone = [CopyNoteProbe]::WaitMenuGone($p.Id, 2000)
  $parked = [CopyNoteProbe]::WaitParked($p.Id, 3000) -ge 0
  Check (($menu -ne [IntPtr]::Zero) -and $gone -and $parked) 'the hotkey puts the window away and closes its menu' "opened $($menu -ne [IntPtr]::Zero), closed $gone, parked $parked"

  # The tray icon's menu, drawn by the same code.
  [void][CopyNoteProbe]::GainForegroundRights()
  [void][CopyNoteProbe]::Post($tray, $TRAY_CALLBACK, 0, $WM_RBUTTONUP)
  $menu = [CopyNoteProbe]::WaitMenu($p.Id, 3000)
  Check (($menu -ne [IntPtr]::Zero) -and ([CopyNoteProbe]::Owner($menu) -eq [IntPtr]::Zero)) "the tray icon's menu still opens, owned by no window"
  if ($menu -ne [IntPtr]::Zero) {
    Send-MenuKeys $menu @($VK_DOWN, $VK_DOWN, $VK_RETURN)
    $shown = [CopyNoteProbe]::WaitOnScreen($p.Id, 3000) -ge 0
    Start-Sleep -Milliseconds 500
    $settings = (Invoke-Page "!!document.querySelector('h1')") -eq 'true'
    Check ($shown -and $settings) 'Down, Down and Enter in it open Settings' "shown $shown, settings $settings"
  }
}
finally { Stop-TestInstance $p }

Complete-Checks
