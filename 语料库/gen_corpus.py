# -*- coding: utf-8 -*-
"""灵思微数测试平台 - 生产环境仿真语料生成器
生成 1K/8K/32K/128K/1M 五档连续中文自然文本（多文档类型混合，禁止重复填充）
"""
import random
import os

random.seed(20260817)

OUT_DIR = os.path.dirname(os.path.abspath(__file__))

# ---------- 素材池 ----------
SURNAMES = list("王李张刘陈杨赵黄周吴徐孙马朱胡郭何高林罗郑梁谢宋唐许韩冯邓曹彭曾肖田董潘袁蔡蒋余杜叶程苏魏吕丁任沈姚卢姜崔钟")
GIVEN1 = list("伟强磊军洋勇杰涛斌鹏宇浩凯旭阳明志建华文辉国庆世嘉子雨思梦天俊文嘉欣怡静淑兰桂英慧巧美娜静淑")
GIVEN2 = list("华军生林飞成龙山川宁安远卓然风帆航晨曦萱琪瑶涵婷芳莉娟霞燕萍红玉兰梅竹菊松涛波峰岩")

CITIES = ["武汉", "上海", "深圳", "杭州", "成都", "北京", "广州", "南京", "苏州", "西安", "长沙", "合肥", "郑州", "青岛", "重庆"]
CHANNELS = ["400热线", "在线客服", "企业微信", "工单系统", "邮件", "小程序留言"]
PRODUCTS = ["QM-100数据采集网关", "QM-200边缘计算网关", "QM-Cloud数据平台", "LW-300温湿度记录仪", "GW-50工业路由器", "SE-8振动传感器"]
ISSUES = ["设备频繁掉线，平均每天断开三到四次", "升级固件后无法连接平台，指示灯常红", "数据上传延迟超过十分钟，影响产线看板", "外壳在运输中出现裂纹，怀疑包装问题", "平台报表数据与现场仪表读数不一致", "夜间定时任务偶发失败，日志显示超时", "新购设备无法通过现有账号绑定", "历史数据导出格式与文档说明不符"]
DEMANDS = ["安排工程师四十八小时内上门排查", "先行补发一台备用机，故障机随后退回", "提供远程诊断并给出书面原因说明", "按合同条款申请换货并承担运费", "补偿因停机造成的损失并出具整改计划", "升级到最新固件并远程协助配置"]
MOODS = ["较为激动", "比较平和但多次强调时限", "明显不满并提到投诉渠道", "理性但要求书面回复"]

PROJECTS = ["会员体系重构", "智慧园区一期", "数据中台迁移", "客服机器人二期", "仓储WMS升级", "边缘网关固件V3", "经销商门户改版", "BI报表平台"]
DEPTS = ["技术部", "市场部", "运营部", "产品部", "客服部", "财务部", "供应链部"]
RISKS = ["第三方接口文档迟迟未提供，联调存在延期风险", "历史数据清洗进度落后计划约百分之二十", "关键岗位一名同事下月休产假，需提前交接", "客户新增需求可能挤占测试时间", "服务器资源使用率已接近八成，需扩容评估"]
ACTIONS = ["输出详细设计文档并组织评审", "完成接口联调并提交测试报告", "整理遗留问题清单并逐项闭环", "与客户确认验收标准和时间表", "补充监控告警规则并观察一周"]

SERVICES = ["pay-svc", "order-svc", "user-center", "gateway", "report-svc", "notify-svc", "auth-svc", "wms-core"]
EVENTS = ["滚动升级完成，新版本{ver}已全量发布", "检测到接口P99延迟升至{lat}毫秒，已触发告警", "自动扩容完成，实例数由{a}增至{b}", "定时对账任务执行成功，处理单据{n}笔", "缓存命中率降至{p}%，已自动切换备用集群", "证书将于十五天后到期，已创建续期工单"]

CLAUSES = ["付款方式", "交付与验收", "知识产权", "保密义务", "违约责任", "不可抗力", "争议解决", "合同变更与解除", "质保与售后", "数据安全"]

def rname():
    return random.choice(SURNAMES) + random.choice(GIVEN1) + (random.choice(GIVEN2) if random.random() < 0.6 else "")

def rdate():
    return f"2026年{random.randint(1,8)}月{random.randint(1,28)}日"

def rdt():
    return f"2026-{random.randint(1,8):02d}-{random.randint(1,28):02d} {random.randint(8,20):02d}:{random.randint(0,59):02d}"

def rmoney():
    return f"{random.uniform(0.5, 300):.2f}万元"

def rcompany():
    return random.choice(["恒创", "瑞丰", "中科", "蓝海", "启明", "智联", "华骏", "天宇", "鑫源", "宏图"]) + random.choice(["制造", "物流", "电子", "食品", "新能源", "汽配", "医药", "包装"]) + "有限公司"

# ---------- 文档生成器 ----------
def gen_ticket():
    return (
        f"【客服工单】编号CS-{random.randint(20260000,20269999)}，提交时间{rdt()}，渠道：{random.choice(CHANNELS)}。"
        f"{random.choice(CITIES)}的{rcompany()}客户{random.choice(['反映','反馈','投诉'])}，其部署的{random.choice(PRODUCTS)}{random.choice(ISSUES)}。"
        f"客户{random.choice(MOODS)}，希望我方{random.choice(DEMANDS)}。"
        f"值班客服{rname()}核实订单号{random.randint(10000000,99999999)}及保修状态后，"
        f"{random.choice(['已创建技术工单转交二线支持','已远程读取设备日志初步定位','已升级为重点工单并通知值班经理'])}，"
        f"并告知客户将在{random.choice(['两','四','二十四'])}小时内给出{random.choice(['初步结论','处理方案','上门时间安排'])}。"
        f"后续跟进人：{rname()}，预计闭环时间：{rdate()}。"
    )

def gen_meeting():
    attendees = "、".join(rname() for _ in range(random.randint(4, 7)))
    items = []
    for i in range(random.randint(3, 5)):
        items.append(
            f"{i+1}. {random.choice(['确认','讨论','评审'])}{random.choice(PROJECTS)}{random.choice(['进度安排','范围边界','资源投入','验收口径','上线计划'])}："
            f"会议决定由{rname()}牵头，{rdate()}前{random.choice(ACTIONS)}。"
        )
    return (
        f"【会议纪要】{random.choice(PROJECTS)}项目{random.choice(['周例会','专题会','启动会','评审会'])}，时间{rdt()}，地点{random.choice(['3F-赤壁会议室','线上腾讯会议','5F-东湖会议室'])}，"
        f"主持人{rname()}，记录人{rname()}，参会：{attendees}。会议形成如下决议：" + "".join(items) +
        f"遗留问题：{random.choice(RISKS)}，由{rname()}跟踪，下次会议{random.choice(['复核进展','专项讨论'])}。"
    )

def gen_weekly():
    p = random.choice(PROJECTS)
    return (
        f"【项目周报】{p}项目（{rdate()}当周）：本周完成整体进度的{random.randint(3,12)}%，累计完成{random.randint(20,95)}%。"
        f"主要进展：{random.choice(ACTIONS)}；{random.choice(ACTIONS)}。"
        f"数据方面：本周新增缺陷{random.randint(2,15)}个，关闭{random.randint(3,18)}个，遗留严重缺陷{random.randint(0,3)}个；"
        f"自动化用例通过率{random.uniform(88,99.5):.1f}%。"
        f"风险与阻塞：{random.choice(RISKS)}。"
        f"下周计划：{random.choice(ACTIONS)}；{random.choice(ACTIONS)}。项目负责人：{rname()}。"
    )

def gen_contract():
    n = random.randint(3, 5)
    picks = random.sample(CLAUSES, n)
    out = [f"【合同节选】甲方：{rcompany()}；乙方：{rcompany()}。合同编号HT-{random.randint(2026000,2026999)}，签订日期{rdate()}。主要条款摘要如下。"]
    for i, c in enumerate(picks, 1):
        if c == "付款方式":
            body = f"合同总价{rmoney()}，预付款百分之{random.choice(['二十','三十'])}于合同生效后{random.randint(5,15)}个工作日内支付，验收合格后支付至百分之九十五，余款作为质保金于质保期满后支付。"
        elif c == "交付与验收":
            body = f"乙方应于合同签订后{random.randint(30,120)}日内完成交付，甲方应在收到交付物后{random.randint(10,30)}日内完成验收，逾期未提出书面异议视为验收合格。"
        elif c == "违约责任":
            body = f"任何一方逾期履行主要义务的，每逾期一日按合同总价千分之{random.randint(1,5)}支付违约金；逾期超过{random.randint(15,45)}日的，守约方有权解除合同。"
        elif c == "争议解决":
            body = f"因本合同产生的争议，双方应友好协商解决；协商不成的，提交{random.choice(CITIES)}仲裁委员会仲裁。"
        else:
            body = f"双方确认，{c}相关事项按行业通行标准执行，具体细则以双方另行签署的补充协议为准，补充协议与本合同具有同等法律效力。"
        out.append(f"第{i}条（{c}）：{body}")
    return "".join(out)

def gen_opslog():
    lines = [f"【运维日志】服务集群{random.choice(['华东','华南','华中'])}生产环境，值班工程师{rname()}。当日关键事件记录如下。"]
    for _ in range(random.randint(4, 7)):
        ev = random.choice(EVENTS).format(
            ver=f"v{random.randint(1,5)}.{random.randint(0,20)}.{random.randint(0,9)}",
            lat=random.randint(200, 3000), a=random.randint(4, 12), b=random.randint(13, 30),
            n=random.randint(1000, 90000), p=f"{random.uniform(70, 92):.1f}")
        lines.append(f"{rdt()} [{random.choice(SERVICES)}] {ev}。")
    lines.append(f"当日整体可用性{random.uniform(99.9,99.999):.3f}%，无{random.choice(['重大','一级'])}事故。")
    return "".join(lines)

def gen_release():
    feats = "；".join(random.sample(["新增设备分组批量配置功能", "报表中心支持自定义时间粒度", "告警规则支持企业微信通知", "开放API新增三个查询接口", "设备地图支持省市区三级钻取", "支持固件灰度升级策略"], 3))
    fixes = "；".join(random.sample(["修复偶发的登录态失效问题", "修复大数据量导出超时问题", "修复部分机型时区显示错误", "修复看板数字千分位缺失问题"], 2))
    return (
        f"【产品公告】QM-Cloud平台V{random.randint(2,5)}.{random.randint(0,9)}版本于{rdate()}发布。"
        f"新增功能：{feats}。问题修复：{fixes}。"
        f"本次升级为不停机发布，预计耗时{random.randint(10,40)}分钟，期间可能出现短暂的功能入口抖动。"
        f"如有异常请通过{random.choice(CHANNELS)}联系值班同事{rname()}。"
    )

def gen_finance():
    m = random.randint(1, 8)
    rev = random.uniform(200, 2600)
    mom = random.uniform(-5, 12)
    mom_txt = f"环比增长{mom:.1f}%" if mom >= 0 else f"环比下降{abs(mom):.1f}%"
    hw = random.randint(40, 60)
    sub = random.randint(20, 90 - hw)
    svc = 100 - hw - sub
    return (
        f"【经营摘要】2026年{m}月经营快报：当月实现营业收入{rev:.1f}万元，"
        f"同比增长{random.uniform(5,35):.1f}%，{mom_txt}；"
        f"毛利率{random.uniform(28,55):.1f}%，较上月{random.choice(['提升','回落'])}{random.uniform(0.2,3):.1f}个百分点。"
        f"分业务看，硬件销售收入占比{hw}%，平台订阅收入占比{sub}%，实施服务收入占比{svc}%。"
        f"当月新签客户{random.randint(5,40)}家，续费金额{rmoney()}，客户续费率{random.uniform(82,96):.1f}%。"
        f"应收账款余额{rmoney()}，账龄九十天以上占比{random.uniform(3,15):.1f}%，财务部已启动专项催收。"
        f"分析人：{rname()}。"
    )

def gen_policy():
    topics = random.sample(["差旅报销", "信息安全", "供应商准入", "合同审批", "印章使用", "数据备份", "客户接待", "采购付款"], 3)
    out = [f"【制度条款】摘自《{random.choice(['公司管理条例','行政管理制度','内控手册'])}》（2026年修订版），生效日期{rdate()}。"]
    base = random.randint(3, 25)
    for i, t in enumerate(topics):
        out.append(
            f"第{base + i}条（{t}）：员工办理{t}相关事项，应提前{random.randint(1,5)}个工作日在OA系统提交申请，"
            f"经直接上级及{random.choice(DEPTS)}负责人审批后执行；涉及金额超过{random.choice(['五千','一万','五万'])}元的，须报分管副总裁审批。"
            f"违反本条的，视情节给予{random.choice(['通报批评','绩效扣分','取消当季评优资格'])}处理。"
        )
    return "".join(out)

def gen_market():
    return (
        f"【市场分析】{random.choice(['工业物联网','边缘计算','智慧仓储','企业服务SaaS'])}行业{random.choice(['月度','季度'])}观察："
        f"{random.choice(CITIES)}及周边区域客户需求保持{random.choice(['平稳增长','快速增长','结构性分化'])}，"
        f"其中{random.choice(['中小制造企业','头部物流园区','连锁零售企业'])}的{random.choice(['数据采集','能耗管理','预测性维护'])}预算明显增加。"
        f"竞品方面，{random.choice(['A厂商','B厂商','某头部厂商'])}近期将主力机型降价约{random.randint(5,20)}%，并推出{random.choice(['以旧换新','三年质保','免费试用九十日'])}政策，"
        f"预计对{random.choice(['中低端','存量替换','新增采购'])}市场形成挤压。"
        f"建议我司在{random.choice(['行业解决方案','交付时效','本地化服务'])}上强化差异竞争，避免正面价格战。分析人：{rname()}。"
    )

def gen_kb():
    return (
        f"【知识库条目】问：{random.choice(PRODUCTS)}{random.choice(['无法上线如何排查','如何恢复出厂设置','数据上传间隔如何修改','如何对接第三方平台'])}？"
        f"答：第一步，确认设备供电与网络连接正常，指示灯{random.choice(['常绿','闪烁'])}为正常状态；"
        f"第二步，登录管理后台检查{random.choice(['接入点配置','证书有效期','固件版本'])}是否为最新；"
        f"第三步，如仍未解决，请导出设备日志并通过{random.choice(CHANNELS)}提交工单，注明设备序列号与采购批次。"
        f"注意事项：操作过程中请勿断电，恢复出厂设置将清除全部本地配置，请提前备份。条目维护人：{rname()}，更新日期{rdate()}。"
    )

GENS = [gen_ticket, gen_meeting, gen_weekly, gen_contract, gen_opslog,
        gen_release, gen_finance, gen_policy, gen_market, gen_kb]

HEADERS = {
    "1K": "本语料为真实业务场景仿真文本，内容节选自某工业物联网企业日常运营档案，用于短上下文输入测试。\n\n",
    "8K": "本语料为真实业务场景仿真文本，汇编某工业物联网企业一周内的客服工单、项目纪要与运维记录，用于8K级上下文输入测试。\n\n",
    "32K": "本语料为真实业务场景仿真文本，汇编某工业物联网企业月度运营档案，涵盖客服、项目、合同、财务、运维等多类原始文档，用于32K级上下文输入测试。\n\n",
    "128K": "本语料为真实业务场景仿真文本，汇编某工业物联网企业季度运营档案，涵盖客服工单、会议纪要、项目周报、合同条款、运维日志、经营数据、管理制度、市场分析与知识库等九类原始文档，用于128K级长上下文输入测试。\n\n",
    "1M": "本语料为真实业务场景仿真文本，汇编某工业物联网企业年度综合运营档案，涵盖客服工单、会议纪要、项目周报、合同条款、运维日志、经营数据、管理制度、市场分析与知识库等九类原始文档，按时间线混合编排，用于百万级超长上下文输入测试。\n\n",
}

def header_for(tag):
    if tag in HEADERS:
        return HEADERS[tag]
    return f"本语料为真实业务场景仿真文本，内容节选自某工业物联网企业日常运营档案，涵盖客服工单、会议纪要、项目周报、合同条款、运维日志、经营数据等多类原始文档，用于{tag}级上下文输入测试。\n\n"

# 1 token ≈ 1.5 字（平台口径）
TARGETS = [
    ("1K", 1500), ("2K", 3000), ("2.5K", 3750), ("3K", 4500), ("4K", 6000),
    ("5K", 7500), ("6K", 9000), ("7K", 10500), ("8K", 12000), ("9K", 13500),
    ("10K", 15000), ("32K", 48000), ("128K", 192000), ("256K", 384000),
    ("512K", 768000), ("1M", 1500000),
]

def build(tag, target):
    text = header_for(tag)
    while len(text) < target:
        text += random.choice(GENS)() + "\n\n"
    # 在目标长度附近找句号截断，保证以完整句子结尾、不超限过多
    cut = text.rfind("。", target - 300, target + 50)
    if cut == -1:
        cut = text.rfind("。", 0, target + 50)
    return text[:cut + 1]

def main():
    for tag, target in TARGETS:
        content = build(tag, target)
        path = os.path.join(OUT_DIR, f"语料-{tag}档.txt")
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        print(f"{tag}: 目标{target}字, 实际{len(content)}字, 文件 {path}")

if __name__ == "__main__":
    main()
