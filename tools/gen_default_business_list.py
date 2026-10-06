# -*- coding: utf-8 -*-
"""从 restock_matched.csv 生成 dnf-robot 默认经营清单 JSON（嵌入 Go 包）。

对齐 dnf-market-agent 的补货清单语义：
- sell_price = system_price（固定卖价，不随机浮动、不按强化加价）
- buy_max_price = sell_price * 85 // 100（按卖价 85% 折扣，保证严格低于卖价，防套利）
- target_quantity = quantity（目标总库存）
- stack_size = stack_size（每条挂单数量，装备为 1）
- upgrade_policy = ignore（忽略强化，按 sell_price 固定价）

按 item_id 去重，保留第一条。字段范围按 validateBusinessItem 钳制。
"""
import csv
import json

CSV_IN = r'D:/code/dnf-robot/restock_matched.csv'
JSON_OUT = r'D:/code/dnf-robot/internal/capability/marketapp/default_business_list.json'

FIELD_NOTES = {
    "item_id": "用于匹配的 DNF 物品 ID。",
    "name": "可选备注名称，仅供管理员识别，不参与匹配。",
    "min_price": "最终最低单价；堆叠物品按单件计算，装备按单条拍卖记录计算。",
    "max_price": "最终最高单价，必须大于或等于 min_price。",
    "enabled": "是否启用该物品的独立价格覆盖。",
    "sell_enabled": "是否补货；target_quantity 为目标总数量，stack_size 为每条数量，装备始终一件。",
    "sell_price": "基础系统出售单价；升级策略为 actual 时按实际强化和全局强化率同时调整买卖价。",
    "buy_max_price": "最高收购单价，不是付款额；必须严格低于所有系统最低售价。",
    "buy_enabled": "是否收购；buy_daily_quantity_limit 必须为正数。",
    "upgrade_policy": "ignore 忽略强化；actual 按挂单实际强化加价，读取不到按 +0，称号等不加价。",
    "limits": "max_actions 每轮上限、max_order_gold 单笔上限、daily_gold 每日金币预算；均必须为正数。",
}

MAX_PRICE = 2_000_000_000
MAX_QTY = 1_000_000


def clamp(v, lo, hi):
    return max(lo, min(v, hi))


items = []
seen = set()
skipped = []
with open(CSV_IN, encoding='utf-8-sig') as f:
    for row in csv.DictReader(f):
        try:
            item_id = int(row['item_id'])
            system_price = int(row['system_price'])
            quantity = int(row['quantity'])
            stack_size = int(row['stack_size'])
        except (ValueError, KeyError):
            continue
        if item_id <= 0 or system_price <= 0 or quantity <= 0 or stack_size <= 0:
            skipped.append((row.get('item_id'), row.get('name'), 'invalid value'))
            continue
        if item_id in seen:
            skipped.append((item_id, row.get('name'), 'duplicate item_id'))
            continue
        seen.add(item_id)
        sell = clamp(system_price, 1, MAX_PRICE)
        qty = clamp(quantity, 1, MAX_QTY)
        stk = clamp(stack_size, 1, MAX_QTY)
        buy = sell
        buy_enabled = buy >= 1
        items.append({
            "item_id": item_id,
            "name": row.get('name', ''),
            "min_price": 0,
            "max_price": 0,
            "enabled": False,
            "sell_enabled": True,
            "sell_price": sell,
            "target_quantity": qty,
            "stack_size": stk,
            "buy_enabled": buy_enabled,
            "buy_max_price": buy if buy_enabled else 0,
            "upgrade_policy": "ignore",
            "buy_daily_quantity_limit": min(qty, MAX_QTY) if buy_enabled else 0,
        })

doc = {
    "description": "拍卖行物品经营清单。单品买卖分价，未配置默认禁收禁售。v1 仅兼容原卖价，不产生收购权限。旧挂单保留，低价历史库存触发禁收。v2 卖价固定，不使用通用随机折扣或装备倍率。",
    "field_notes": FIELD_NOTES,
    "version": 2,
    "limits": {"max_actions": 100, "max_order_gold": 50000000, "daily_gold": 100000000},
    "items": items,
}

with open(JSON_OUT, 'w', encoding='utf-8') as f:
    json.dump(doc, f, ensure_ascii=False, separators=(',', ':'))

buy_count = sum(1 for it in items if it['buy_enabled'])
print(f"生成 {len(items)} 条到 {JSON_OUT}")
print(f"开启收购 {buy_count} 条，仅上架 {len(items) - buy_count} 条")
print(f"跳过 {len(skipped)} 条（无效值或重复 item_id）")
if skipped:
    print("跳过明细前 10 条：")
    for s in skipped[:10]:
        print(f"  {s}")