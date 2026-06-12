#!/usr/bin/env python3
"""调研摘要生成器：扫描 references/research/ 目录统计调研结果。

用法: python3 references/merge_research.py <skill目录>

扫描 01-06.md 文件，统计：来源数量、一手/二手占比、关键发现。
输出 Phase 1.5 检查点的 markdown 表格。
"""

import re
from pathlib import Path
from typing import Optional
import sys


def count_lines(filepath: Path) -> int:
    try:
        return len(filepath.read_text().splitlines())
    except Exception:
        return 0


def extract_from_file(filepath: Path) -> dict:
    """从单个调研文件中提取统计信息。"""
    if not filepath.exists():
        return {"lines": 0, "sources": 0, "primary": 0, "findings": "", "agent": filepath.stem}

    text = filepath.read_text()
    lines = text.count("\n") + 1

    # Count source mentions (URLs + explicit citations)
    urls = len(re.findall(r'https?://\S+', text))
    citations = len(re.findall(r'来源[：:]|Source:|出自|引自|来源URL', text))
    sources = max(urls, citations, 1)

    # Estimate primary vs secondary
    primary_terms = r'一手|原始|本人|本人著作|本人撰写|原始文本|原话|原著|公开演讲|本人.*书|本人.*说'
    secondary_terms = r'二手|转述|别人总结|据.*报道|引用.*来源|传记|他人分析|书评|公众号'

    primary = len(re.findall(primary_terms, text))
    secondary = len(re.findall(secondary_terms, text))

    # Key findings: first ## section or first 3 bullet points
    findings = ""
    bullets = re.findall(r'^[-*]\s+(.+)', text, re.MULTILINE)
    if bullets:
        findings = "; ".join(b[:60] for b in bullets[:3])

    return {
        "lines": lines,
        "sources": sources,
        "primary": primary,
        "secondary": secondary,
        "findings": findings,
        "agent": filepath.stem,
    }


def scan_directory(skill_dir: Path) -> dict:
    research_dir = skill_dir / "references" / "research"
    if not research_dir.exists():
        return None

    agents = {}
    agent_labels = {
        "01-writings": "1 著作",
        "02-conversations": "2 对话",
        "03-expression-dna": "3 表达",
        "04-external-views": "4 他者",
        "05-decisions": "5 决策",
        "06-timeline": "6 时间线",
    }

    for filename, label in agent_labels.items():
        fpath = research_dir / f"{filename}.md"
        info = extract_from_file(fpath)
        info["label"] = label
        agents[label] = info

    return agents


def main():
    if len(sys.argv) < 2:
        print("用法: python3 merge_research.py <skill目录>")
        sys.exit(1)

    skill_dir = Path(sys.argv[1])
    agents = scan_directory(skill_dir)

    if not agents:
        print("未找到 references/research/ 目录或文件为空")
        sys.exit(1)

    print(f"\n调研质量摘要: {skill_dir.name}")
    print("-" * 70)
    print(f"| Agent            | 行数 | 来源数 | 一手/二手 | 关键发现 |")
    print(f"|------------------|------|--------|----------|----------|")

    total_sources = 0
    total_primary = 0
    total_secondary = 0
    conflicts = 0

    for label in ["1 著作", "2 对话", "3 表达", "4 他者", "5 决策", "6 时间线"]:
        a = agents.get(label)
        if a and a["lines"] > 0:
            ratio = f"{a['primary']}/{a['secondary']}"
            findings = a["findings"][:60] if a["findings"] else "-"
            print(f"| {label:<16.12} | {a['lines']:>4} | {a['sources']:>5} | {ratio:>7} | {findings:<30.30} |")
            total_sources += a["sources"]
            total_primary += a["primary"]
            total_secondary += a["secondary"]
        else:
            print(f"| {label:<16.12} |    0 |     0 |       - | (缺失) |")

    # Count conflicts (look for矛盾 in 04-external-views)
    external_path = skill_dir / "references" / "research" / "04-external-views.md"
    if external_path.exists():
        conflicts = external_path.read_text().count("矛盾") + external_path.read_text().count("冲突")

    print(f"|------------------|------|--------|----------|----------|")
    print(f"| **总计**         |  -   | {total_sources:>5} | {total_primary}/{total_secondary} | - |")
    print(f"| 矛盾点           | {conflicts}处  | -        | -        | - |")
    print()

    # Check for missing dimensions
    missing = [label for label in ["1 著作", "2 对话", "3 表达", "4 他者", "5 决策", "6 时间线"]
               if agents[label]["lines"] == 0]
    if missing:
        print(f"⚠️ 信息不足维度: {', '.join(missing)}")
    else:
        print("✅ 所有维度均已覆盖")


if __name__ == "__main__":
    main()
