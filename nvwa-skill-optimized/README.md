<div align="center">

# 女娲 · Skill造人术

> *「你想蒸馏的下一个员工，何必是同事」*

让乔布斯、马斯克、芒格、费曼都给你打工。输入一个名字，自动调研→提炼→生成可运行的人物思维框架。

[English](#english) · [人物列表](CHARACTERS.md) · [示例质量](examples/README.md)

</div>

---

## 快速开始

```bash
npx skills add xmg2024/nvwa-skill
```

```text
> 蒸馏一个保罗·格雷厄姆
> 造一个张小龙的视角Skill
> 用芒格的视角帮我分析这个投资决策
> 切换到乔布斯，这个产品要怎么取舍
```

## 效果一览

| 你问 | Ta 答 |
|------|------|
| OpenAI 和 Anthropic 谁的方向对？ | 你问错了。这不是关于方向的竞赛，这是关于品味的竞赛。——乔布斯 |
| 同时做三件事，精力不够 | 不是精力不够，是合同太多。哪一个做起来你会忘记时间？——Naval |
| 获客成本太高了 | 不是优化漏斗，是质疑漏斗本身该不该存在。——马斯克 |

## 女娲提取什么（五层认知）

| 层级 | 内容 | 例子 |
|------|------|------|
| 怎么说话 | 表达DNA——语气、节奏、用词偏好 | 芒格的「铁锤人倾向」、乔布斯的「这是狗屎」 |
| 怎么想 | 心智模型、认知框架 | 马斯克的第一性原理、塔勒布的反脆弱 |
| 怎么判断 | 决策启发式 | 「如果一件事 80% 的人都做，反向操作」 |
| 什么不做 | 价值观底线 | 费曼：绝不假装理解 |
| 知道局限 | 诚实边界 | 孙割：我搞不了技术，但我能搞流量 |

## 工作原理（4 步流水线）

1. **六路并行采集** — 6 个 Agent 同时搜索著作、对话、社交媒体、批评者、决策记录、时间线
2. **三重验证提炼** — 跨域复现（≥2 个不同领域）+ 生成力（能推断新问题立场）+ 排他性（不是所有聪明人都这么想）
3. **构建 Skill** — 3-7 个心智模型 + 5-10 条决策启发式 + 完整表达DNA
4. **质量验证** — 已知测试（3 题）+ 边缘测试（1 题）+ 风格测试，通过才交付

> 完整方法论：`references/extraction-framework.md`

## 包含的人物（15 个）

| 🔥 = 独立仓库可安装 | ⭐ = 有完整示例对话 |

14 个人物 + 1 个主题 Skill。详见 [CHARACTERS.md](CHARACTERS.md)。质量详情见 [examples/README.md](examples/README.md)。

## 质量工具

| 工具 | 用途 |
|------|------|
| `quality_check.py` | 自动校验生成的 SKILL.md（6 项标准） |
| `merge_research.py` | 自动生成 Phase 1.5 调研统计表 |
| `.github/workflows/validate.yml` | CI 自动校验所有示例 |

```bash
# 质量自检
python3 references/quality_check.py examples/steve-jobs-perspective/SKILL.md

# 调研统计
python3 references/merge_research.py examples/steve-jobs-perspective
```

## English

Nvwa distills anyone's cognitive framework — mental models, decision heuristics, expression DNA — into a runnable skill. Not role-playing. Cognitive architecture extraction.

```bash
npx skills add xmg2024/nvwa-skill
```

15 skills included (14 people + 1 topic). MIT License.

---

MIT © 小码哥 xmg2024
