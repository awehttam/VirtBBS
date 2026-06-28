#!/usr/bin/env python3
"""Convert VirtBBS HTML templates to Bootstrap 5 patterns."""

import re
import sys
from pathlib import Path

TABLE_CLASS = 'table table-dark table-striped table-hover align-middle mb-0'
SKIP_FILES = {'layout.html'}


def panel_classes(match: re.Match) -> str:
    extra = match.group(1) or ''
    extra = extra.replace('panel', '').replace('login-box', '').strip()
    parts = ['card', 'mb-3'] + [p for p in extra.split() if p]
    return f'class="{" ".join(parts)}"'


def convert_panels(content: str) -> str:
    content = re.sub(r'class="(?:login-box\s+)?panel([^"]*)"', panel_classes, content)
    return content


def extract_card_blocks(content: str) -> str:
    """Wrap card div contents with card-header (first h2/h3) and card-body."""
    lines = content.split('\n')
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        m = re.match(r'^(\s*)<div class="card mb-3[^"]*">$', line)
        if not m:
            out.append(line)
            i += 1
            continue
        indent = m.group(1)
        inner = []
        depth = 1
        i += 1
        while i < len(lines) and depth > 0:
            cur = lines[i]
            depth += cur.count('<div')
            depth -= cur.count('</div>')
            if depth > 0:
                inner.append(cur)
            i += 1

        header_idx = None
        for j, ln in enumerate(inner):
            if re.match(r'\s*<h[23](\s|>)', ln):
                header_idx = j
                break

        out.append(f'{indent}<div class="{re.search(r"class=\"([^\"]+)\"", line).group(1)}">')
        if header_idx is not None:
            hline = inner[header_idx]
            hm = re.match(r'^(\s*)<(h[23])([^>]*)>(.*)</\2>\s*$', hline)
            if hm:
                out.append(f'{indent}  <div class="card-header"><{hm.group(2)} class="h5 mb-0"{hm.group(3)}>{hm.group(4)}</{hm.group(2)}></div>')
            else:
                out.append(f'{indent}  <div class="card-header">{hline.strip()}</div>')
            body_lines = inner[:header_idx] + inner[header_idx + 1:]
        else:
            body_lines = inner

        if body_lines:
            out.append(f'{indent}  <div class="card-body">')
            out.extend(body_lines)
            out.append(f'{indent}  </div>')
        out.append(f'{indent}</div>')
    return '\n'.join(out)


def wrap_tables(content: str) -> str:
    lines = content.split('\n')
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        tm = re.match(r'^(\s*)<table(\s[^>]*)?>$', line) or re.match(r'^(\s*)<table>$', line)
        if tm and (i == 0 or 'table-responsive' not in lines[i - 1]):
            indent = tm.group(1)
            out.append(f'{indent}<div class="table-responsive">')
            if 'class="' in line:
                line = re.sub(
                    r'class="([^"]*)"',
                    lambda m: f'class="{TABLE_CLASS}"' if TABLE_CLASS.split()[0] not in m.group(1) else m.group(0),
                    line,
                )
                if TABLE_CLASS not in line:
                    line = line.replace('class="', f'class="{TABLE_CLASS} ', 1)
            else:
                line = line.replace('<table', f'<table class="{TABLE_CLASS}"', 1)
            out.append(line)
            i += 1
            while i < len(lines):
                out.append(lines[i])
                if re.match(r'^\s*</table>\s*$', lines[i]):
                    out.append(f'{indent}</div>')
                    i += 1
                    break
                i += 1
            continue
        out.append(line)
        i += 1
    return '\n'.join(out)


def convert_grids(content: str) -> str:
    content = content.replace(
        '<div class="stats-grid">',
        '<div class="row row-cols-2 row-cols-md-4 g-2">',
    )
    content = re.sub(
        r'<div class="stat-card">',
        '<div class="col"><div class="card text-center"><div class="card-body py-2">',
        content,
    )
    content = content.replace('</span></div>', '</span></div></div></div>', 1)  # fragile
    # fix stat-card closes: each stat-card line is self-contained
    content = re.sub(
        r'(<span class="stat-label">[^<]*</span></div>)',
        r'\1</div></div>',
        content,
    )

    content = content.replace(
        '<div class="menu-grid">',
        '<div class="row row-cols-1 row-cols-sm-2 row-cols-lg-3 g-3">',
    )
    content = re.sub(
        r'<div class="menu-card">',
        '<div class="col"><div class="card h-100 text-center"><div class="card-body">',
        content,
    )
    content = re.sub(
        r'(<div class="col"><div class="card h-100 text-center"><div class="card-body">\s*\n\s*<a href="[^"]*">[^<]*(?:<span[^>]*>[^<]*</span>)?</a>\s*\n\s*<p>[^<]*</p>\s*\n\s*)</div>',
        r'\1</div></div></div>',
        content,
    )
    # menu-card lines are usually: col>card>card-body> a + p, close with </div></div></div>
    lines = content.split('\n')
    out = []
    menu_depth = 0
    for line in lines:
        if '<div class="col"><div class="card h-100 text-center"><div class="card-body">' in line:
            menu_depth = 3
            out.append(line)
            continue
        if menu_depth > 0 and re.match(r'\s*</div>\s*$', line) and menu_depth == 1:
            out.append(line)
            out.append(re.match(r'^(\s*)', line).group(1) + '</div></div>')
            menu_depth = 0
            continue
        if menu_depth > 0 and re.match(r'\s*<p>', line):
            menu_depth = 1
        out.append(line)
    content = '\n'.join(out)

    return content


def convert_menu_cards(content: str) -> str:
    """Close menu-card col wrappers properly."""
    lines = content.split('\n')
    out = []
    in_menu_card = False
    for line in lines:
        if '<div class="col"><div class="card h-100 text-center"><div class="card-body">' in line:
            in_menu_card = True
            out.append(line)
            continue
        if in_menu_card and re.match(r'^\s*</div>\s*$', line):
            indent = re.match(r'^(\s*)', line).group(1)
            out.append(line)
            out.append(f'{indent}</div></div>')
            in_menu_card = False
            continue
        out.append(line)
    return '\n'.join(out)


def convert_netmail(content: str) -> str:
    content = content.replace('class="netmail-layout"', 'class="row g-3"')
    content = re.sub(r'<aside([^>]*)>', r'<aside\1 class="col-12 col-md-4 col-lg-3">', content)
    content = re.sub(r'<section([^>]*)>', r'<section\1 class="col-12 col-md-8 col-lg-9">', content)
    # fix double class on aside/section if class already present
    content = re.sub(r'class="([^"]*)" class="', r'class="\1 ', content)
    content = re.sub(
        r'<aside id="netmail-list" class="col-12 col-md-4 col-lg-3">',
        '<aside id="netmail-list" class="col-12 col-md-4 col-lg-3">',
        content,
    )
    content = re.sub(
        r'<section id="netmail-pane" class="col-12 col-md-8 col-lg-9">',
        '<section id="netmail-pane" class="col-12 col-md-8 col-lg-9">',
        content,
    )
    return content


def convert_alerts(content: str) -> str:
    content = re.sub(r'class="error"', 'class="error alert alert-danger"', content)
    content = re.sub(r'class="flash"', 'class="flash alert alert-success"', content)
    return content


def convert_badges(content: str) -> str:
    content = re.sub(r'class="badge"(?! bg-)', 'class="badge bg-primary"', content)
    return content


def convert_search_forms(content: str) -> str:
    def fix_form(m: re.Match) -> str:
        cls = m.group(1)
        if 'search-form' in cls:
            if 'd-flex' not in cls:
                cls = cls + ' d-flex flex-wrap gap-2 align-items-center mb-3'
        else:
            cls = cls + ' search-form d-flex flex-wrap gap-2 align-items-center mb-3'
        return f'class="{cls.strip()}"'

    content = re.sub(r'class="([^"]*search-form[^"]*)"', fix_form, content)
    content = re.sub(
        r'<form method="get" action="/search"(?!\s+class)',
        '<form method="get" action="/search" class="search-form d-flex flex-wrap gap-2 align-items-center mb-3"',
        content,
    )
    return content


def add_form_control(tag: str, kind: str) -> str:
    if kind == 'select':
        if 'class="' in tag:
            if 'form-select' not in tag:
                tag = re.sub(r'class="([^"]*)"', r'class="form-select \1"', tag)
        else:
            tag = tag.replace('<select', '<select class="form-select"', 1)
    elif kind == 'textarea':
        if 'class="' in tag:
            if 'form-control' not in tag:
                tag = re.sub(r'class="([^"]*)"', r'class="form-control \1"', tag)
        else:
            tag = tag.replace('<textarea', '<textarea class="form-control"', 1)
    else:
        if 'class="' in tag:
            if 'form-control' not in tag:
                tag = re.sub(r'class="([^"]*)"', r'class="form-control \1"', tag)
        else:
            tag = tag.replace('<input', '<input class="form-control"', 1)
    return tag


def convert_forms(content: str) -> str:
    skip_input_types = {'hidden', 'checkbox', 'radio', 'submit', 'button', 'file'}

    def input_repl(m: re.Match) -> str:
        tag = m.group(0)
        tm = re.search(r'type="([^"]+)"', tag)
        if tm and tm.group(1) in skip_input_types:
            return tag
        if 'form-control' in tag or 'form-select' in tag:
            return tag
        return add_form_control(tag, 'input')

    content = re.sub(r'<input[^>]+>', input_repl, content)
    content = re.sub(r'<select[^>]*>', lambda m: add_form_control(m.group(0), 'select'), content)
    content = re.sub(r'<textarea[^>]*>', lambda m: add_form_control(m.group(0), 'textarea'), content)

    # wrap label[for] + following input/select/textarea in mb-3 div
    lines = content.split('\n')
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        lm = re.match(r'^(\s*)<label\s+for="([^"]+)"([^>]*)>(.*)</label>\s*$', line)
        if lm and i + 1 < len(lines):
            nxt = lines[i + 1]
            if re.match(r'^\s*<(input|select|textarea)\b', nxt) and 'mb-3' not in line:
                indent = lm.group(1)
                out.append(f'{indent}<div class="mb-3">')
                out.append(f'{indent}  <label for="{lm.group(2)}"{lm.group(3)} class="form-label">{lm.group(4)}</label>')
                out.append(nxt)
                out.append(f'{indent}</div>')
                i += 2
                continue
        # label on own line, input next line (no for attr on same pattern)
        lm2 = re.match(r'^(\s*)<label\s+for="([^"]+)"([^>]*)>\s*$', line)
        if lm2 and i + 1 < len(lines):
            nxt = lines[i + 1]
            if re.match(r'^\s*<(input|select|textarea)\b', nxt):
                indent = lm2.group(1)
                out.append(f'{indent}<div class="mb-3">')
                out.append(line.replace('<label', '<label class="form-label"', 1) if 'class=' not in line else line)
                out.append(nxt)
                # consume until /label or next field
                i += 2
                if i < len(lines) and re.match(r'^\s*</label>\s*$', lines[i]):
                    out.append(lines[i])
                    i += 1
                out.append(f'{indent}</div>')
                continue
        out.append(line)
        i += 1
    return '\n'.join(out)


def convert_buttons(content: str) -> str:
    def btn_repl(m: re.Match) -> str:
        tag = m.group(0)
        if 'type="submit"' in tag or 'type="button"' in tag or tag.startswith('<button'):
            if 'btn-link' in tag:
                tag = re.sub(r'class="[^"]*"', 'class="btn btn-link"', tag)
                if 'class="' not in tag:
                    tag = tag.replace('<button', '<button class="btn btn-link"', 1)
            elif 'btn-sm' in tag and 'btn btn-primary' not in tag:
                tag = re.sub(r'class="[^"]*"', 'class="btn btn-primary btn-sm"', tag)
                if 'class="' not in tag:
                    tag = tag.replace('<button', '<button class="btn btn-primary btn-sm"', 1)
            elif 'btn btn-primary' not in tag:
                if 'class="btn"' in tag:
                    tag = tag.replace('class="btn"', 'class="btn btn-primary"')
                elif 'class="' in tag and 'btn' not in tag:
                    pass
                else:
                    tag = tag.replace('<button', '<button class="btn btn-primary"', 1) if 'class="' not in tag else re.sub(r'class="([^"]*)"', r'class="btn btn-primary \1"', tag)
        return tag

    content = re.sub(r'<button[^>]*>.*?</button>', btn_repl, content, flags=re.DOTALL)
    content = re.sub(r'<button([^>]*)>', btn_repl, content)
    content = re.sub(r'<a class="btn"(?!\s)', '<a class="btn btn-primary"', content)
    content = re.sub(r'<a class="btn btn-sm"', '<a class="btn btn-primary btn-sm"', content)
    return content


def convert_standalone_login(content: str, filename: str) -> str:
    if filename != 'login.html':
        return content
    if 'bootstrap.min.css' in content:
        return content
    content = content.replace(
        '<link rel="stylesheet" href="/static/style.css">',
        '<link rel="stylesheet" href="/static/bootstrap.min.css">\n  <link rel="stylesheet" href="/static/style.css">',
    )
    content = content.replace(
        '<body>',
        '<body class="bg-dark d-flex align-items-center min-vh-100 py-4">',
    )
    content = content.replace(
        '<div class="card mb-3">',
        '<div class="container"><div class="card mx-auto shadow" style="max-width:420px">',
        1,
    )
    # close container before body end
    content = content.replace(
        '</div>\n</body>',
        '</div></div>\n<script src="/static/bootstrap.bundle.min.js"></script>\n</body>',
        1,
    )
    return content


def convert_auth_pages(content: str, filename: str) -> str:
    """Bootstrap card wrapper for register/forgot/reset when using login-box pattern."""
    if filename in ('register.html', 'forgot_password.html', 'reset_password.html'):
        content = content.replace('register-box', '').replace('login-box', '')
    return content


def process_file(path: Path) -> str:
    content = path.read_text(encoding='utf-8')
    if path.name in SKIP_FILES:
        return content

    content = convert_panels(content)
    content = extract_card_blocks(content)
    content = convert_grids(content)
    content = convert_menu_cards(content)
    content = convert_netmail(content)
    content = convert_alerts(content)
    content = convert_badges(content)
    content = convert_search_forms(content)
    content = convert_forms(content)
    content = convert_buttons(content)
    content = wrap_tables(content)
    content = convert_standalone_login(content, path.name)
    content = convert_auth_pages(content, path.name)
    return content


def main() -> None:
    dirs = [Path(sys.argv[1]), Path(sys.argv[2])] if len(sys.argv) > 2 else [
        Path('internal/web/defaults/templates'),
        Path('www/templates'),
    ]
    changed = []
    for d in dirs:
        for path in sorted(d.glob('*.html')):
            if path.name in SKIP_FILES:
                continue
            new = process_file(path)
            old = path.read_text(encoding='utf-8')
            if new != old:
                path.write_text(new, encoding='utf-8')
                changed.append(str(path))
    print(f'Updated {len(changed)} files:')
    for f in changed:
        print(f'  {f}')


if __name__ == '__main__':
    main()
