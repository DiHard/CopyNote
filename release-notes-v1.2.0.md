## CopyNote v1.2.0

### New features
- **Drag-and-drop reordering of entries.** Hold and drag a card to move it within the list — surrounding cards smoothly shift around it. The regular click still copies; a drag only starts after the pointer travels ~5 px, so single-clicks are never misinterpreted. Reordering also works from the keyboard with **Ctrl + ↑ / Ctrl + ↓** on a focused card.
- **Compact Settings layout.** Padding inside rows and gaps between sections shrunk by ~17–25 %, so the Settings panel takes noticeably less vertical space.

### Internal
- New `Reorder` service method on the Go side, with validation (length, duplicates, unknown ids) and atomic on-disk persistence. Covered by tests.
