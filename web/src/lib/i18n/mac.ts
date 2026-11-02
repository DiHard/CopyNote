/**
 * Wording that differs on a Mac: the menu bar instead of the tray, macOS
 * instead of Windows, Finder instead of Explorer. Only the keys that change
 * are here; everything else comes from the ordinary dictionaries.
 */
export const mac: Record<string, Record<string, string>> = {
  en: {
    "header.hide": "Hide window",
    "list.empty.tray":
      "The app lives in the menu bar, top right. Click its icon to open this window again.",
    "settings.autorun.hint":
      "CopyNote starts in the menu bar when you log in, without opening its window.",
    "settings.autohide.hint":
      "Turn this off to keep the window up while you work in another app. Close it with the ✕, Esc, or the menu bar icon.",
    "settings.topmost.hint":
      "With hiding turned off, the window stays in front of the form you are filling in.",
    "settings.theme.hint": "System follows the macOS light or dark appearance.",
    "settings.language.hint":
      "System uses the macOS language if the app has it, English otherwise.",
    "settings.openFolder": "Show in Finder",
    "copyError.busy": "Could not copy. Try again.",
  },
  ru: {
    "header.hide": "Скрыть окно",
    "list.empty.tray":
      "Приложение живёт в строке меню вверху справа. Нажмите на его значок, чтобы снова открыть это окно.",
    "settings.autorun.hint":
      "После входа в систему CopyNote запустится в строке меню, не открывая окно.",
    "settings.autohide.hint":
      "Выключите, чтобы окно оставалось на экране, пока вы работаете в другой программе. Закрыть его можно крестиком, Esc или значком в строке меню.",
    "settings.topmost.hint":
      "Вместе с выключенным скрытием окно не потеряется за формой, которую вы заполняете.",
    "settings.theme.hint":
      "«Системная» меняется вместе с оформлением macOS — светлым или тёмным.",
    "settings.language.hint":
      "«Системный» берёт язык macOS, если он поддерживается, а иначе английский.",
    "settings.openFolder": "Показать в Finder",
    "copyError.busy": "Не удалось скопировать. Попробуйте ещё раз.",
  },
};
