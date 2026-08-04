#!/usr/bin/env python3
"""Генератор иконок Music Orchestrator.

Рисует ноту программно (Pillow) и раскладывает по всем местам, где она нужна:
favicon, apple-touch-icon для «Добавить на экран Домой», PWA-иконки для React
и для Flutter Web, плюс мастер 1024×1024 для нативных сборок iOS и Android.

Почему кодом, а не картинкой из редактора: иконка должна пересобираться при
смене акцентного цвета одной командой, а не перерисовываться руками в десяти
размерах. Запуск: python3 tools/make_icons.py
"""

from __future__ import annotations

import math
import os
from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Плитка светлая, знак тёмный: на домашнем экране среди тёмных иконок
# лаймовый квадрат читается издалека, а тёмная нота даёт максимальный контраст.
LIME_TOP = (206, 255, 106)
LIME_BOTTOM = (150, 214, 40)
INK = (11, 13, 18)

# Мастер рисуется с четырёхкратным запасом и уменьшается — так края ноты
# получаются гладкими без отдельного сглаживания.
SUPERSAMPLE = 4


def _gradient(size: int) -> Image.Image:
    """Диагональный градиент — плоская заливка выглядит дёшево на большой плитке."""
    img = Image.new("RGB", (size, size))
    px = img.load()
    for y in range(size):
        for x in range(size):
            t = (x / size * 0.35) + (y / size * 0.65)
            px[x, y] = tuple(
                round(LIME_TOP[i] + (LIME_BOTTOM[i] - LIME_TOP[i]) * t) for i in range(3)
            )
    return img


def _note_mask(size: int) -> Image.Image:
    """Восьмая нота: головка, штиль, флажок.

    Головка наклонена — у прямой ноты силуэт читается как запятая.
    """
    s = size
    mask = Image.new("L", (s, s), 0)
    draw = ImageDraw.Draw(mask)

    head_w, head_h = int(s * 0.305), int(s * 0.232)
    head = Image.new("L", (head_w, head_h), 0)
    ImageDraw.Draw(head).ellipse((0, 0, head_w - 1, head_h - 1), fill=255)
    head = head.rotate(20, resample=Image.BICUBIC, expand=True)

    head_cx, head_cy = int(s * 0.400), int(s * 0.712)
    mask.paste(head, (head_cx - head.width // 2, head_cy - head.height // 2), head)

    stem_w = max(1, int(s * 0.058))
    stem_x = head_cx + int(head_w * 0.5) - stem_w
    stem_top, stem_bottom = int(s * 0.245), head_cy
    draw.rounded_rectangle(
        (stem_x, stem_top, stem_x + stem_w, stem_bottom),
        radius=stem_w // 2,
        fill=255,
    )

    # Флажок — «толстая дуга» из двух парабол с заливкой между ними.
    # Толщина падает лишь до 76% от начальной. Сильнее сужать нельзя: тонкий
    # хвост сначала читается как отдельная точка, а на мелких размерах исчезает.
    steps = 200
    start_x = stem_x                    # вровень со штилем, иначе на стыке зазубрина
    start_y = stem_top + stem_w * 0.5   # ниже скруглённой шапки штиля
    span_x, span_y = s * 0.200, s * 0.250
    t0, t1 = s * 0.100, s * 0.076
    outer, inner = [], []
    for i in range(steps + 1):
        t = i / steps
        x = start_x + span_x * math.sin(t * math.pi / 2)
        y = start_y + span_y * t * t
        # Внутренний контур смещается по нормали к кривой, а не по вертикали:
        # на крутом нижнем участке вертикальный сдвиг даёт почти нулевую
        # ширину, и хвост вырождается в остриё.
        dx = span_x * (math.pi / 2) * math.cos(t * math.pi / 2)
        dy = span_y * 2 * t
        n = math.hypot(dx, dy) or 1.0
        nx, ny = -dy / n, dx / n
        thick = t0 + (t1 - t0) * t
        outer.append((x, y))
        inner.append((x + nx * thick, y + ny * thick))
    draw.polygon(outer + list(reversed(inner)), fill=255)

    return mask


def build_master(size: int = 1024) -> Image.Image:
    big = size * SUPERSAMPLE
    tile = _gradient(big)
    ink = Image.new("RGB", (big, big), INK)
    tile.paste(ink, (0, 0), _note_mask(big))
    return tile.resize((size, size), Image.LANCZOS)


def rounded(img: Image.Image, radius_ratio: float = 0.225) -> Image.Image:
    """Скруглённая версия для тех мест, где система не режет углы сама."""
    size = img.width
    mask = Image.new("L", (size * 4, size * 4), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        (0, 0, size * 4 - 1, size * 4 - 1),
        radius=int(size * 4 * radius_ratio),
        fill=255,
    )
    mask = mask.resize((size, size), Image.LANCZOS)
    out = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    out.paste(img.convert("RGBA"), (0, 0), mask)
    return out


def maskable(master: Image.Image, size: int) -> Image.Image:
    """PWA-маска обрезает до 20% с краёв, поэтому знак ужимается в safe zone."""
    out = Image.new("RGB", (size, size), LIME_TOP)
    inner = int(size * 0.72)
    out.paste(master.resize((inner, inner), Image.LANCZOS), ((size - inner) // 2,) * 2)
    return out


def save(img: Image.Image, *parts: str) -> None:
    path = os.path.join(ROOT, *parts)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    img.save(path)
    print(f"  {os.path.relpath(path, ROOT)}  {img.width}×{img.height}")


def main() -> None:
    master = build_master(1024)

    print("мастер:")
    save(master, "assets", "icon-1024.png")

    print("React (frontend/frontend/public):")
    web = ("frontend", "frontend", "public")
    save(rounded(master.resize((180, 180), Image.LANCZOS)), *web, "apple-touch-icon.png")
    for px in (16, 32, 192, 512):
        save(master.resize((px, px), Image.LANCZOS), *web, f"icon-{px}.png")
    save(maskable(master, 512), *web, "icon-maskable-512.png")
    master.resize((32, 32), Image.LANCZOS).save(
        os.path.join(ROOT, *web, "favicon.ico"), sizes=[(16, 16), (32, 32), (48, 48)]
    )
    print("  frontend/frontend/public/favicon.ico")

    print("Flutter Web (mobile/web):")
    fw = ("mobile", "web")
    save(master.resize((16, 16), Image.LANCZOS), *fw, "favicon.png")
    save(rounded(master.resize((180, 180), Image.LANCZOS)), *fw, "apple-touch-icon.png")
    for px in (192, 512):
        save(master.resize((px, px), Image.LANCZOS), *fw, "icons", f"Icon-{px}.png")
        save(maskable(master, px), *fw, "icons", f"Icon-maskable-{px}.png")

    print("\nГотово. Нативные iOS/Android собираются из assets/icon-1024.png:")
    print("  cd mobile && dart run flutter_launcher_icons")


if __name__ == "__main__":
    main()
