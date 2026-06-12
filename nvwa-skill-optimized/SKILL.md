---
name: huashu-nvwa
description: |
  女娲造人：输入人名/主题/甚至只是模糊需求，自动深度调研→思维框架提炼→生成可运行的人物Skill。
  两种入口：(1)明确人名→直接蒸馏 (2)模糊需求→诊断推荐→再蒸馏。
  触发词：「造skill」「蒸馏XX」「女娲」「造人」「XX的思维方式」「做个XX视角」「更新XX的skill」。
  模糊需求也触发：「我想提升决策质量」「有没有一种思维方式能帮我...」「我需要一个思维顾问」。
---

# 女娲 · Skill造人术

> 「写不进去的那部分，才是你真正的护城河。」——但写得进去的部分，已经足够强大。

## 核心理念

女娲不是复制人，是**提炼思维框架**。一个好的人物Skill是一套可运行的认知操作系统：心智模型、决策启发式、表达DNA、反模式、诚实边界。

**关键区分**：捕捉的是 HOW they think，不是 WHAT they said。

---
## 执行流程（5 Phase + 2 检查点）

### Phase 0: 入口分流

| 用户输入 | 路径 | 示例 |
|---------|------|------|
| 明确的人名/主题 | **直接路径** → Phase 0A | 「蒸馏芒格」 |
| 模糊的需求/困惑 | **诊断路径** → Phase 0B | 「我想提升决策质量」 |

### Phase 0A: 需求澄清（直接路径）
确认：人名/聚焦方向/用途/新建or更新/本地语料。详见 `references/research-archive.md`。

### Phase 0B: 需求诊断（模糊路径）
通过1-2个追问定位需求维度 → 推荐2-3个候选 → 用户选择。需求维度表和推荐格式详见 `references/research-archive.md`。

### Phase 0.5: 创建Skill目录（立即执行）
在 `.claude/skills/[name]-perspective/` 下创建目录结构，详见 `references/research-archive.md`。

### Phase 1: 多源信息采集（6路并行Agent）
启动6个并行subagent，每个负责不同信息维度。任务分配表、prompt模板、工具辅助详见 `references/research-archive.md`。

硬性要求：结果写入 `references/research/0X-xxx.md`，标注来源可信度，保留矛盾。

#### 检查点 1.5: 调研Review
展示调研质量摘要给用户确认。用 `python3 references/merge_research.py <skill目录>` 自动生成统计表格。

### Phase 2: 框架提炼
读取 `references/extraction-framework.md` 获取三重验证方法论。提炼心智模型(3-7个)、决策启发式(5-10条)、表达DNA、价值观、诚实边界。详见 `references/research-archive.md`。

#### 检查点 2.5: 提炼确认
展示提炼摘要给用户确认。

### Phase 3: Skill构建
读取 `references/skill-template.md` 模板，填充内容生成 SKILL.md。包含回答工作流(Agentic Protocol)，详见 `references/research-archive.md`。

### Phase 4: 质量验证 + 精炼后置

生成Skill后，用子agent执行3项测试：已知测试(3题)、边缘测试(1题)、风格测试(100字)。

通过标准详见 `references/quality-checklist.md`。

**迭代上限2轮**。2轮后仍有不通过项→标注薄弱维度，交付当前最优版本。

**精炼后置（3条规则）**：
1. 检查触发条件是否覆盖真实使用场景
2. 检查边界条件是否写清楚（什么问题这个Skill不该回答）
3. 干跑1个测试prompt验证输出质量

用 `python3 references/quality_check.py <SKILL.md路径>` 自动运行6项标准检查。

**展示验证结果给用户确认后才算完成。**

---
## 更新已有Skill
只启动 Agent 2+5+6，增量更新，详见 `references/research-archive.md`。

---
## 品味守则（速查）

| 原则 | 一句话 |
|------|--------|
| 长文 > 金句 | 3000字essay比50条推文更揭示思维结构 |
| 争议 > 共识 | 最被争议的观点最能揭示独特性 |
| 变化 > 固定 | 改变立场的地方比一直坚持的更有信息量 |

### 绝不做的事
- 编造此人没说过的话
- 把通用道理包装成此人的「独特见解」
- 忽略负面评价和争议
- 在信息不足时强行生成

---
## 特殊场景

- **活人 vs 历史人物**：活人注意时效性；历史人物多源交叉验证
- **主题Skill vs 人物Skill**：变体表详见 `references/research-archive.md`
- **中国人物 vs 西方人物**：中国→B站/小宇宙/36氪/晚点/财新（知乎和微信公众号永远排除）；西方→Twitter/YouTube/Podcast
- **冷门人物**：来源<10条时降级处理
- **蒸馏用户自己**：需要用户提供素材

---
## 最后

女娲造的不是人，是一面镜子。一个好的人物Skill，让你用另一个人的眼睛看自己的问题。不是为了模仿他们，而是为了拓展你自己的思维边界。
