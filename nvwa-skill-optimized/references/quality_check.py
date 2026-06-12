#!/usr/bin/env python3
"""Quality check script for nvwa-skill generated SKILL.md files.

Usage: python3 references/quality_check.py <SKILL.md>
"""

import re
import sys
from pathlib import Path


def load_frontmatter(path):
    text = Path(path).read_text()
    m = re.match(r'^---\s*\n(.*?)\n---', text, re.DOTALL)
    if not m:
        return {}
    data = {}
    for line in m.group(1).strip().split("\n"):
        if ":" in line:
            k, v = line.split(":", 1)
            data[k.strip()] = v.strip()
    return data


def count_sections(text):
    sections = {
        "models": len(re.findall(r'^#{2,3}\s+.*?\d{1,2}[:\s]', text, re.MULTILINE)),
        "model_evidence": len(re.findall(r'\*\*.*证据\*\*|\*\*Evidence\*\*', text)),
        "decisions": len(re.findall(r'^[-\d]+[.)]\s+\*\*', text, re.MULTILINE)),
        "expression": 1 if re.search(r'表达DNA|Expression DNA|表达风格|风格规则', text) else 0,
        "honesty": 1 if re.search(r'诚实边界|Honesty|局限|Limitations', text) else 0,
        "limitations": len(re.findall(r'局限|Limitation|失效|不适用|做不到|cannot', text)),
        "tension": len(re.findall(r'内在张力|矛盾|冲突|没想清楚|Tension', text)),
    }
    return sections


def estimate_primary_ratio(text):
    primary = [
        r'biography', r'speech', r'interview', r'podcast', r'keynote',
        r'primary source', r'original text', r'his own words', r'her own words',
    ]
    secondary = [
        r'secondary', r'analysis by', r'reported by', r'according to',
    ]
    p = sum(1 for pat in primary if re.search(pat, text, re.IGNORECASE))
    s = sum(1 for pat in secondary if re.search(pat, text, re.IGNORECASE))
    total = p + s
    if total == 0:
        return 0.5
    return min(1.0, p / total)


def check(path):
    text = Path(path).read_text()
    sections = count_sections(text)
    ratio = estimate_primary_ratio(text)
    fm = load_frontmatter(path)

    result = {
        "path": path,
        "fm_valid": bool(fm.get("name")),
        "models": sections["models"],
        "models_ok": 3 <= sections["models"] <= 10,
        "evidence_ok": sections["model_evidence"] >= max(1, sections["models"] * 0.5),
        "expression_ok": sections["expression"] == 1,
        "honesty_ok": sections["limitations"] >= 3,
        "tension_ok": sections["tension"] >= 2,
        "primary_ratio": ratio,
        "primary_ok": ratio > 0.5,
        "decisions": sections["decisions"],
    }
    result["passed"] = all([
        result["fm_valid"], result["models_ok"], result["evidence_ok"],
        result["expression_ok"], result["honesty_ok"],
        result["tension_ok"], result["primary_ok"],
    ])
    return result


def main():
    if len(sys.argv) < 2:
        print("Usage: python3 quality_check.py <SKILL.md>")
        sys.exit(1)

    path = sys.argv[1]
    if not Path(path).exists():
        print(f"File not found: {path}")
        sys.exit(1)

    r = check(path)
    print(f"\nQuality Report: {path}")
    print("=" * 50)
    print(f"  Frontmatter: {'PASS' if r['fm_valid'] else 'FAIL'}")
    print(f"  Models: {r['models']} (3-7={r['models_ok']}) {'PASS' if r['models_ok'] else 'FAIL'}")
    print(f"  Evidence: {'PASS' if r['evidence_ok'] else 'FAIL'}")
    print(f"  Expression DNA: {'PASS' if r['expression_ok'] else 'FAIL'}")
    print(f"  Honesty (>=3 limits): {'PASS' if r['honesty_ok'] else 'FAIL'}")
    print(f"  Tension (>=2): {'PASS' if r['tension_ok'] else 'FAIL'}")
    print(f"  Primary source ratio: {r['primary_ratio']:.0%} (>50%={r['primary_ok']}) {'PASS' if r['primary_ok'] else 'FAIL'}")
    print(f"  Decisions: {r['decisions']}")
    print("=" * 50)
    print(f"  Overall: {'PASS' if r['passed'] else 'FAIL'}")
    print()

    sys.exit(0 if r["passed"] else 1)


if __name__ == "__main__":
    main()
