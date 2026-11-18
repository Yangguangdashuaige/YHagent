# -*- coding: utf-8 -*-
"""gen_md_preview.py — 把大 Markdown 生成为带侧边目录/搜索/滚动定位的单文件 HTML 预览。

用法：
    python docs/gen_md_preview.py                 # 默认生成 docs/面试.md -> docs/面试-预览.html
    python docs/gen_md_preview.py docs/迭代历史.md  # 指定任意 md 文件

md 更新后重跑一次即可刷新 HTML。
"""
import html
import re
import sys
from datetime import datetime
from pathlib import Path

import markdown

ROOT = Path(__file__).resolve().parent.parent

TEMPLATE = """<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{title} · 预览</title>
<style>
:root{{
  --navy:#1C3D5A; --navy-2:#2B5A80; --gold:#B68A35;
  --bg:#FBFCFE; --surface:#FFFFFF; --border:#DFE5EC;
  --text-1:#222831; --text-2:#334155; --text-3:#64748B;
}}
*{{box-sizing:border-box}}
body{{margin:0;font-family:"Microsoft YaHei UI","Microsoft YaHei","PingFang SC",sans-serif;color:var(--text-1);background:var(--bg);font-size:15px}}
#layout{{display:flex;min-height:100vh}}

/* ===== 侧边目录 ===== */
#sidebar{{position:fixed;top:0;left:0;bottom:0;width:320px;background:var(--surface);border-right:1px solid var(--border);display:flex;flex-direction:column;z-index:10}}
#sidebar header{{padding:16px 18px 10px;border-bottom:1px solid var(--border)}}
#sidebar header .t{{font-weight:700;font-size:15px;color:var(--navy);margin-bottom:4px;word-break:break-all}}
#sidebar header .m{{font-size:11px;color:var(--text-3)}}
#search{{margin:12px 14px 8px;padding:8px 12px;border:1px solid var(--border);border-radius:8px;font-size:13px;outline:none;width:calc(100% - 28px)}}
#search:focus{{border-color:var(--gold)}}
#search-info{{font-size:11px;color:var(--text-3);padding:0 18px 6px}}
#toc{{overflow-y:auto;flex:1;padding:4px 8px 24px}}
#toc a{{display:block;padding:5px 10px;color:var(--text-2);text-decoration:none;font-size:13px;border-radius:6px;line-height:1.45;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}}
#toc a:hover{{background:var(--bg);color:var(--navy)}}
#toc a.lv1{{font-weight:700;color:var(--navy);margin-top:10px}}
#toc a.lv2{{padding-left:22px}}
#toc a.lv3{{padding-left:40px;font-size:12px;color:var(--text-3)}}
#toc a.active{{background:var(--navy);color:#fff}}
#toc a.hidden{{display:none}}

/* ===== 正文 ===== */
#main{{margin-left:320px;flex:1;padding:36px 56px 120px}}
#main .doc{{max-width:920px;margin:0 auto}}
#main h1,#main h2,#main h3,#main h4{{color:var(--navy);scroll-margin-top:20px}}
#main h1{{font-size:28px;border-bottom:2px solid var(--gold);padding-bottom:8px}}
#main h2{{font-size:22px;margin-top:40px;border-left:5px solid var(--gold);padding-left:12px}}
#main h3{{font-size:18px;margin-top:28px}}
#main table{{border-collapse:collapse;width:100%;margin:14px 0;background:var(--surface);font-size:13.5px}}
#main th,#main td{{border:1px solid var(--border);padding:7px 11px;text-align:left;vertical-align:top;line-height:1.6}}
#main th{{background:#F1F5F9;color:var(--navy)}}
#main tr:nth-child(even) td{{background:#FAFBFD}}
#main blockquote{{margin:12px 0;padding:8px 16px;border-left:4px solid var(--gold);background:var(--surface);color:var(--text-2)}}
#main code{{background:#F1F5F9;padding:1px 6px;border-radius:4px;font-size:13px}}
#main pre{{background:#12283C;color:#E8F0F8;padding:14px 18px;border-radius:10px;overflow-x:auto;font-size:13px;line-height:1.6}}
#main pre code{{background:none;padding:0}}
#main a{{color:var(--navy-2)}}
#main hr{{border:none;border-top:1px solid var(--border);margin:28px 0}}
#main img{{max-width:100%}}

/* ===== 返回顶部 ===== */
#topbtn{{position:fixed;right:28px;bottom:28px;width:44px;height:44px;border-radius:50%;background:var(--navy);color:#fff;border:none;font-size:18px;cursor:pointer;box-shadow:0 4px 14px rgba(28,61,90,.35);display:none;z-index:20}}
#topbtn:hover{{background:var(--gold)}}

@media (max-width:900px){{
  #sidebar{{display:none}}
  #main{{margin-left:0;padding:24px}}
}}
</style>
</head>
<body>
<div id="layout">
  <nav id="sidebar">
    <header>
      <div class="t">{title}</div>
      <div class="m">生成于 {gen_time} · 由 gen_md_preview.py 生成</div>
    </header>
    <input id="search" type="text" placeholder="搜索目录（如：rerank / 记忆 / 八股）…">
    <div id="search-info"></div>
    <div id="toc">{toc}</div>
  </nav>
  <main id="main"><div class="doc">{body}</div></main>
</div>
<button id="topbtn" title="返回顶部">↑</button>
<script>
// —— 目录搜索过滤 ——
const box=document.getElementById('search'),info=document.getElementById('search-info');
const links=[...document.querySelectorAll('#toc a')];
box.addEventListener('input',()=>{{
  const q=box.value.trim().toLowerCase();let n=0;
  links.forEach(a=>{{
    const hit=!q||a.textContent.toLowerCase().includes(q);
    a.classList.toggle('hidden',!hit);if(hit)n++;
  }});
  info.textContent=q?`匹配 ${{n}} / ${{links.length}} 条`:`共 ${{links.length}} 条目录`;
}});
info.textContent=`共 ${{links.length}} 条目录`;
box.addEventListener('keydown',e=>{{if(e.key==='Enter'&&links.length){{const v=links.find(a=>!a.classList.contains('hidden'));if(v)v.click();}}}});

// —— 滚动定位：当前阅读位置在目录中高亮 ——
const heads=[...document.querySelectorAll('#main h1[id],#main h2[id],#main h3[id]')];
const idmap={{}};links.forEach(a=>{{const h=a.getAttribute('href');if(h)idmap[h.slice(1)]=a;}});
function spy(){{
  let cur=null;const y=window.scrollY+120;
  for(const h of heads){{if(h.offsetTop<=y)cur=h;}}
  links.forEach(a=>a.classList.remove('active'));
  if(cur&&idmap[cur.id]){{idmap[cur.id].classList.add('active');idmap[cur.id].scrollIntoView({{block:'nearest'}});}}
}}
let t=null;window.addEventListener('scroll',()=>{{document.getElementById('topbtn').style.display=window.scrollY>600?'block':'none';clearTimeout(t);t=setTimeout(spy,80);}});

// —— 平滑跳转 ——
document.getElementById('toc').addEventListener('click',e=>{{
  const a=e.target.closest('a');if(!a)return;e.preventDefault();
  const el=document.querySelector(a.getAttribute('href'));
  if(el)el.scrollIntoView({{behavior:'smooth',block:'start'}});
}});

// —— 返回顶部 ——
document.getElementById('topbtn').addEventListener('click',()=>window.scrollTo({{top:0,behavior:'smooth'}}));
// '/' 聚焦搜索
document.addEventListener('keydown',e=>{{if(e.key==='/'&&document.activeElement!==box){{e.preventDefault();box.focus();}}}});
</script>
</body>
</html>
"""


def build(src: Path) -> Path:
    text = src.read_text(encoding="utf-8")
    body = markdown.markdown(text, extensions=["tables", "fenced_code"])

    # 渲染后的 HTML 标题注入锚点 id，同时收集目录
    headings: list[tuple[int, str]] = []
    seq = [0]

    def _inject(m: re.Match) -> str:
        lvl, inner = m.group(1), m.group(2)
        plain = re.sub(r"<[^>]+>", "", inner).strip()
        seq[0] += 1
        hid = f"h-{seq[0]}"
        headings.append((len(lvl), plain))
        return f'<h{lvl} id="{hid}">{inner}</h{lvl}>'

    body = re.sub(r"<h([123])>(.*?)</h\1>", _inject, body, flags=re.S)

    toc_items = []
    for lvl, txt in headings:
        toc_items.append(
            f'<a class="lv{lvl}" href="#h-{len(toc_items) + 1}" title="{html.escape(txt)}">{html.escape(txt)}</a>'
        )
    toc = "\n".join(toc_items)

    out = src.with_name(f"{src.stem}-预览.html")
    out.write_text(
        TEMPLATE.format(
            title=html.escape(src.stem),
            gen_time=datetime.now().strftime("%Y-%m-%d %H:%M"),
            toc=toc,
            body=body,
        ),
        encoding="utf-8",
    )
    print(f"OK {src} -> {out}（{len(headings)} 条目录）")
    return out


if __name__ == "__main__":
    target = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "docs" / "面试.md"
    if not target.is_absolute():
        target = (Path.cwd() / target).resolve()
    build(target)
