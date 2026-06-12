<div align="center">

# 女娲.skill

> *「你想蒸馏的下一个员工，何必是同事」*

[English below](#english)

女娲帮你蒸馏任何人的思维方式，让乔布斯、马斯克、芒格、费曼都给你打工。

只需输入一个名字，女娲自动完成调研、提炼、验证全流程。

[人物列表](CHARACTERS.md) · [效果示例](#效果示例) · [安装](#安装) · [工作原理](#工作原理)

---

## 效果示例

**用户**：我同时想做自媒体、写书、做独立开发，精力完全不够

**Naval**：你列了三个欲望。每个欲望都是你跟不快乐签的一份合同。不是精力不够，是合同太多。问你自己：哪一个做起来你会忘记时间？

**用户**：OpenAI和Anthropic谁的方向是对的？

**乔布斯**：你问错了。这不是关于方向的竞赛，这是关于品味的竞赛。

[更多示例 →](CHARACTERS.md)

---

## 安装

```bash
npx skills add xmg2024/nvwa-skill
```

然后在 Claude Code 里：
```
> 蒸馏一个保罗·格雷厄姆
> 造一个张小龙的视角Skill
> 用芒格的视角帮我分析这个投资决策
```

---

## 蒸馏了什么

女娲提取五层：怎么说话（表达DNA）、怎么想（心智模型）、怎么判断（决策启发式）、什么不做（反模式）、知道局限（诚实边界）。

13 个人物 + 1 个主题，详见 [CHARACTERS.md](CHARACTERS.md)。

---

## 工作原理

1. **六路并行采集** — 6个Agent同时跑，各自存档
2. **三重验证提炼** — 跨域复现 + 生成力 + 排他性
3. **构建Skill** — 心智模型 + 决策启发式 + 表达DNA + 诚实边界
4. **质量验证** — 已知测试 + 边缘测试 + 风格测试

---

## English

Nvwa extracts cognitive frameworks (mental models, decision heuristics, expression DNA) from any public figure into a runnable perspective skill. Not role-playing. Cognitive architecture extraction.

Install: `npx skills add xmg2024/nvwa-skill`

13 person skills + 1 topic skill included. MIT License.

---

MIT License © 小码哥 xmg2024
