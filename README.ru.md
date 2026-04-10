# CopyNote

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte&logoColor=white)](https://svelte.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![WebView2](https://img.shields.io/badge/WebView2-Runtime-0078D4?logo=microsoftedge&logoColor=white)](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Легковесная утилита для Windows, которая хранит часто используемые текстовые фрагменты (email, адреса, ID, шаблоны) и копирует их в буфер обмена одним кликом.

[English version](README.md)

## Возможности

- **Копирование в один клик** &mdash; нажмите на запись, и её значение окажется в буфере обмена
- **Управление записями** &mdash; создание, редактирование, удаление и переупорядочивание
- **Мгновенный поиск** &mdash; фильтрация по названию или значению прямо при вводе
- **Интеграция с треем** &mdash; живёт в области уведомлений, открывается левым кликом
- **Автоскрытие при потере фокуса** &mdash; кликните в другое окно, и CopyNote тихо свернётся
- **Единственный экземпляр** &mdash; повторный запуск поднимает уже работающее окно
- **Адаптивная иконка** &mdash; автоматически переключается между светлым и тёмным вариантом при смене темы Windows
- **Портативное** &mdash; один `.exe`, установка не требуется
- **Лёгкое** &mdash; ~7 МБ бинарник, ~40 МБ RAM в простое

## Требования

- **Windows 10 (1809+) или Windows 11**
- **WebView2 Runtime** &mdash; предустановлен в Windows 11 и большинстве обновлённых Windows 10. Если отсутствует, скачайте [Evergreen Bootstrapper](https://developer.microsoft.com/en-us/microsoft-edge/webview2/#download).

## Быстрый старт

Скачайте `copynote.exe` из [Releases](https://github.com/DiHard/CopyNote/releases) и запустите. Всё &mdash; ни установщика, ни зависимостей кроме WebView2.

Приложение стартует свёрнутым в трей. Левый клик по иконке в трее &mdash; открыть окно.

## Сборка из исходников

### Необходимо

| Инструмент | Версия |
|-----------|--------|
| [Go](https://go.dev/dl/) | 1.23+ |
| [Node.js](https://nodejs.org/) | 18+ (только для сборки фронтенда) |

### Шаги

```bash
# 1. Клонировать
git clone https://github.com/DiHard/CopyNote.git
cd CopyNote

# 2. Собрать фронтенд (Svelte + Tailwind → один HTML-файл)
cd web
npm install
npm run build
cd ..

# 3. Собрать Go-бинарник
go build -ldflags="-H=windowsgui -s -w" -o copynote.exe .
```

Полученный `copynote.exe` полностью самодостаточен (фронтенд встроен через `//go:embed`).

### Перегенерация иконки

Иконка трея/exe генерируется из кода (контур Lucide "copy"):

```bash
go run tools/genicon/main.go          # создаёт assets/icon-dark.ico + icon-light.ico
./bin/rsrc.exe -ico "assets/icon-dark.ico,assets/icon-light.ico" -arch amd64 -o resource_windows_amd64.syso
```

## Хранение данных

| Что | Где |
|-----|-----|
| Записи (JSON) | `%APPDATA%\CopyNote\data.json` |
| Кэш WebView2 | `%LOCALAPPDATA%\CopyNote\WebView2\` |

Рядом с исполняемым файлом ничего не хранится &mdash; можно положить куда угодно.

## Технологический стек

| Слой | Технология |
|------|-----------|
| Бэкенд | Go (stdlib + [go-webview2](https://github.com/jchv/go-webview2)) |
| Фронтенд | Svelte 5, TypeScript, Tailwind CSS 4 |
| Сборщик | Vite + vite-plugin-singlefile |
| UI-хост | Microsoft Edge WebView2 |
| Системная интеграция | Win32 API через `golang.org/x/sys/windows` (без cgo) |

## Горячие клавиши

| Клавиша | Контекст | Действие |
|---------|----------|----------|
| `Escape` | Главное окно | Скрыть в трей |
| `Escape` | Любой диалог | Закрыть диалог |
| `Enter` | Форма создания/редактирования | Сохранить |
| `Enter` | Подтверждение удаления | Подтвердить |
| `Tab` | Список записей | Навигация между записями |

## Лицензия

[MIT](LICENSE)
