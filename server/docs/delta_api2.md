# DIY卡战备 - 自定义凑战备

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv3_diy:
    get:
      summary: DIY卡战备 - 自定义凑战备
      deprecated: false
      description: |-
        **32核 112G内存 物理机**为您提供计算服务【超高成本】。

        资源扣除逻辑：
        - 请求并数据正常【10次】资源
        - 请求失败【1次】资源

        要求
        - 战备值不能超过78w，战备值最低为1500。请至少选择一项。
        - 一般3秒内出结果
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: zb
          in: query
          description: 要计算的战备价值，最低 1500 最高780000
          required: false
          example: 10000
          schema:
            type: integer
        - name: exchange
          in: query
          description: 是否使用部门兑换  0禁止  1启用
          required: false
          example: 0
          schema:
            type: integer
        - name: is_bb
          in: query
          description: 背包  0禁止  1启用
          required: false
          example: 1
          schema:
            type: integer
        - name: is_gun
          in: query
          description: 枪械和配件  0禁止  1启用
          required: false
          example: 1
          schema:
            type: integer
        - name: is_gun2_off
          in: query
          description: 关闭双枪 0禁止 1启用
          required: false
          example: 0
          schema:
            type: integer
        - name: is_hj
          in: query
          description: 护甲  0禁止  1启用
          required: false
          example: 1
          schema:
            type: integer
        - name: is_sq
          in: query
          description: 手枪  0禁止  1启用
          required: false
          example: 1
          schema:
            type: integer
        - name: is_tk
          in: query
          description: 头盔  0禁止  1启用
          required: false
          example: 1
          schema:
            type: integer
        - name: is_xg
          in: query
          description: 胸挂  0禁止  1启用
          required: false
          example: 1
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      cz:
                        type: integer
                      data:
                        type: array
                        items:
                          type: object
                          properties:
                            bl:
                              type: integer
                            exchange:
                              type: string
                            exchange_plus:
                              type: 'null'
                            grade:
                              type: integer
                            id:
                              type: integer
                            jiazhang:
                              type: integer
                            jz:
                              type: integer
                              description: 总战备价值
                            name:
                              type: string
                              description: 配装名称
                            pic:
                              type: string
                            price:
                              type: integer
                              description: 花费价格
                            type:
                              type: string
                          required:
                            - bl
                            - exchange
                            - exchange_plus
                            - grade
                            - id
                            - jiazhang
                            - jz
                            - name
                            - pic
                            - price
                            - type
                      jz:
                        type: integer
                      name:
                        type: string
                      price:
                        type: integer
                    required:
                      - cz
                      - data
                      - jz
                      - name
                      - price
                required:
                  - code
                  - message
                  - msg
                  - data
              examples:
                '1':
                  summary: 成功示例
                  value:
                    code: 0
                    message: 成功
                    msg: 成功
                    data:
                      cz: -48930
                      data:
                        - bl: 8
                          exchange: ''
                          exchange_plus: null
                          grade: 1
                          id: 510
                          jiazhang: 0
                          jz: 11158
                          name: Vector冲锋枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18020000003.png
                          price: 140795
                          type: 枪械1
                        - bl: -21
                          exchange: ''
                          exchange_plus: null
                          grade: 4
                          id: 308
                          jiazhang: 1
                          jz: -27780
                          name: Vector共振一体式枪托
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13040000123.png
                          price: 103086
                          type: 枪械1-枪托
                        - bl: -16
                          exchange: ''
                          exchange_plus: null
                          grade: 3
                          id: 351
                          jiazhang: 1
                          jz: -19656
                          name: Vector扩容CMAG70发
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13120000273.png
                          price: 97823
                          type: 枪械1-弹匣
                        - bl: -10
                          exchange: ''
                          exchange_plus: null
                          grade: 3
                          id: 309
                          jiazhang: 1
                          jz: -2974
                          name: DAS 防弹头盔 (几乎全新)
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11010003003.png
                          price: 25188
                          type: 头盔
                        - bl: -14
                          exchange: ''
                          exchange_plus: null
                          grade: 3
                          id: 144
                          jiazhang: 1
                          jz: -6116
                          name: 射手战术背心 (几乎全新)
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11050003004.png
                          price: 37417
                          type: 护甲
                        - bl: 0
                          exchange: ''
                          exchange_plus: null
                          grade: 1
                          id: 602
                          jiazhang: 0
                          jz: 23
                          name: 斜挎包
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11080001001.png
                          price: 2750
                          type: 背包
                        - bl: -10
                          exchange: ''
                          exchange_plus: null
                          grade: 4
                          id: 226
                          jiazhang: 1
                          jz: -4945
                          name: 突击者战术背心
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11070004002.png
                          price: 44011
                          type: 胸挂
                      jz: 501360
                      name: 自定义
                      price: 451070
                '2':
                  summary: 成功示例
                  value:
                    code: 0
                    message: 成功
                    msg: 成功
                    data:
                      - grade: 3
                        name: 416轻型枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000112.png
                        price: 10222
                        price_hour: 1703
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: MRGS镂空枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000183.png
                        price: 10152
                        price_hour: 1692
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: okp-7反射瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000061.png
                        price: 7136
                        price_hour: 1189
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 0
                        name: AKM突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000006.png
                        price: 27105
                        price_hour: 1129
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 0
                        name: P90冲锋枪
                        period: 48
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18020000002.png
                        price: 32562
                        price_hour: 678
                        primaryClass: gun
                        secondClass: gunSMG
                        secondClassCN: 冲锋枪
                      - grade: 0
                        name: M249轻机枪
                        period: 20
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18040000002.png
                        price: -10122
                        price_hour: -506
                        primaryClass: gun
                        secondClass: gunLMG
                        secondClassCN: 轻机枪
                      - grade: 3
                        name: 416稳固枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000113.png
                        price: 13529
                        price_hour: 2254
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 2
                        name: 俄式准直二倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000042.png
                        price: 7851
                        price_hour: 1308
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 3
                        name: 微型红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000062.png
                        price: 8400
                        price_hour: 1400
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置战斗红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000079.png
                        price: 8425
                        price_hour: 1404
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: M7实用消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000196.png
                        price: 5644
                        price_hour: 806
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 幻影垂直握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000033.png
                        price: -22608
                        price_hour: -5652
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 3
                        name: QJB201轻机枪
                        period: 20
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18040000004.png
                        price: 4700
                        price_hour: 235
                        primaryClass: gun
                        secondClass: gunLMG
                        secondClassCN: 轻机枪
                      - grade: 4
                        name: 影袭导轨枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000007.png
                        price: -6952
                        price_hour: -1158
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: Osight微型瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000087.png
                        price: 15063
                        price_hour: 2510
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: RK-0前握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000029.png
                        price: 9560
                        price_hour: 2390
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: SR-3M紧凑突击步枪
                        period: 48
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18020000008.png
                        price: 56193
                        price_hour: 1170
                        primaryClass: gun
                        secondClass: gunSMG
                        secondClassCN: 冲锋枪
                      - grade: 4
                        name: UR特种战术枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000110.png
                        price: 4003
                        price_hour: 667
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: 侧置Osight微型瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000088.png
                        price: 14017
                        price_hour: 2336
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 炽火抑制器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000159.png
                        price: 5872
                        price_hour: 838
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 共振人体工程握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000034.png
                        price: 15230
                        price_hour: 3807
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: X25U斜侧战斗握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000035.png
                        price: 10132
                        price_hour: 2533
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 3
                        name: 微型瞄具增高架
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13210000012.png
                        price: 15412
                        price_hour: 2568
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 3
                        name: 全景红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000043.png
                        price: 23295
                        price_hour: 3882
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 3
                        name: XCOG突击3.5倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000053.png
                        price: 4828
                        price_hour: 804
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 光学狙击8倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000056.png
                        price: -1905
                        price_hour: -317
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 先进多口径消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000208.png
                        price: -9224
                        price_hour: -1317
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 战术三角握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000027.png
                        price: 1034
                        price_hour: 258
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: M4A1突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000001.png
                        price: 10351
                        price_hour: 431
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 3
                        name: QCQ171冲锋枪
                        period: 48
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                        price: 30698
                        price_hour: 639
                        primaryClass: gun
                        secondClass: gunSMG
                        secondClassCN: 冲锋枪
                      - grade: 3
                        name: XRO快速反应瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000044.png
                        price: 8316
                        price_hour: 1386
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 6
                        name: 先进白热成像战斗瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000067.png
                        price: -89726
                        price_hour: -14954
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置微型红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000078.png
                        price: -5058
                        price_hour: -843
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 轻语战术消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000164.png
                        price: -5120
                        price_hour: -731
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: K1精英斜角握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000036.png
                        price: 5001
                        price_hour: 1250
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: LA-3C绿色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000006.png
                        price: 8378
                        price_hour: 1861
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: OLIGHT Odin S战术手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000016.png
                        price: -80764
                        price_hour: -17947
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 3
                        name: 闪电导轨枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000006.png
                        price: 4360
                        price_hour: 726
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: 战斗红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000063.png
                        price: 10147
                        price_hour: 1691
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 共振二代前握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000039.png
                        price: 552
                        price_hour: 138
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: DBAL-X2紫色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000010.png
                        price: 8082
                        price_hour: 1796
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 0
                        name: AUG突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000015.png
                        price: 47019
                        price_hour: 1959
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 3
                        name: 侵掠核心枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000108.png
                        price: 8909
                        price_hour: 1484
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: 影袭托芯枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000184.png
                        price: 8245
                        price_hour: 1374
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: ACOG精准六倍镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000089.png
                        price: 20408
                        price_hour: 3401
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 灵眼3/7测距狙击瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000091.png
                        price: 24159
                        price_hour: 4026
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 竞赛阻手器
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000024.png
                        price: 9236
                        price_hour: 2309
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 破晓垂直手电握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000041.png
                        price: -23183
                        price_hour: -5795
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: OLIGHT WARRIOR 3S战术手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000018.png
                        price: 11906
                        price_hour: 2645
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 3
                        name: 多用途战术增高架
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13210000013.png
                        price: 15521
                        price_hour: 2586
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 0
                        name: SCAR-H战斗步枪
                        period: 30
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000021.png
                        price: 40087
                        price_hour: 1336
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 0
                        name: 腾龙突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000038.png
                        price: 29413
                        price_hour: 1225
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 4
                        name: 3/7可调倍率狙击镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000057.png
                        price: 30439
                        price_hour: 5073
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 伸缩脚架握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000038.png
                        price: -25500
                        price_hour: -6375
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 蜂网遮光罩
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13370000001.png
                        price: 433
                        price_hour: 96
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 0
                        name: M14射手步枪
                        period: 30
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000005.png
                        price: 15618
                        price_hour: 520
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
                      - grade: 4
                        name: RK3后握把
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13030000169.png
                        price: 8530
                        price_hour: 1066
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 4
                        name: 视点3倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000073.png
                        price: 19477
                        price_hour: 3246
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置全景红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000076.png
                        price: 129
                        price_hour: 21
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置XRO快速反应瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000077.png
                        price: -662
                        price_hour: -110
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 战术垂直握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000032.png
                        price: -4270
                        price_hour: -1067
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: CR棱镜阻手器
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000040.png
                        price: 9542
                        price_hour: 2385
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 3
                        name: 枢机稳固枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000107.png
                        price: 9851
                        price_hour: 1641
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: HAMR组合瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000051.png
                        price: 10973
                        price_hour: 1828
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: LPVO多倍率战斗瞄具
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000069.png
                        price: 22350
                        price_hour: 3725
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 钛金竞赛制退器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000160.png
                        price: 6345
                        price_hour: 906
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 沙暴垂直补偿器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000168.png
                        price: 6033
                        price_hour: 861
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 平衡握把底座
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13330000002.png
                        price: -4507
                        price_hour: -563
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 0
                        name: PKM通用机枪
                        period: 20
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18040000001.png
                        price: 7316
                        price_hour: 365
                        primaryClass: gun
                        secondClass: gunLMG
                        secondClassCN: 轻机枪
                      - grade: 0
                        name: PSG-1射手步枪
                        period: 16
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000031.png
                        price: 32195
                        price_hour: 2012
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
                      - grade: 4
                        name: 堡垒水平补偿器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000167.png
                        price: 10415
                        price_hour: 1487
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 密令斜角握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000030.png
                        price: -11441
                        price_hour: -2860
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: AK-12突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000018.png
                        price: 33805
                        price_hour: 1408
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 3
                        name: Cobra准直式瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                        price: 7998
                        price_hour: 1333
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 6/12神射手变倍狙击镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000070.png
                        price: -11140
                        price_hour: -1856
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: PEQ-2红色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000005.png
                        price: 7880
                        price_hour: 1751
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: 薪火螺旋消焰器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000209.png
                        price: 11098
                        price_hour: 1585
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 黎明三角手电握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000042.png
                        price: -2601
                        price_hour: -650
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: SVD狙击步枪
                        period: 16
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000004.png
                        price: 31141
                        price_hour: 1946
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
                      - grade: 4
                        name: 侵袭后握把
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13030000112.png
                        price: -1198
                        price_hour: -149
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 4
                        name: 1p-29俄制3倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000072.png
                        price: -3885
                        price_hour: -647
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: M157火控光学系统
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000082.png
                        price: 15059
                        price_hour: 2509
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 海神消焰器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000165.png
                        price: 216
                        price_hour: 30
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 死寂消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000169.png
                        price: 10654
                        price_hour: 1522
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 相位战斗前握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000022.png
                        price: 770
                        price_hour: 192
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 斜角阻手器
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000026.png
                        price: -2411
                        price_hour: -602
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 稳固握把底座
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13330000001.png
                        price: 18038
                        price_hour: 2254
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 0
                        name: M700狙击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18060000009.png
                        price: 31635
                        price_hour: 1318
                        primaryClass: gun
                        secondClass: gunSniper
                        secondClassCN: 狙击步枪
                      - grade: 3
                        name: 骨架狙击枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000185.png
                        price: 12151
                        price_hour: 2025
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: 侦察1.5/5可调瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000052.png
                        price: 28482
                        price_hour: 4747
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 6
                        name: 先进热融合全息瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000066.png
                        price: -30910
                        price_hour: -5151
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 耀斑战术手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000007.png
                        price: -95238
                        price_hour: -21164
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: PERST-7蓝色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000009.png
                        price: 8247
                        price_hour: 1832
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: OLIGHT Baldr Pro R多功能手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000014.png
                        price: -76019
                        price_hour: -16893
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 0
                        name: SKS射手步枪
                        period: 32
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000006.png
                        price: 27520
                        price_hour: 860
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-436706942-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 实时卡战备V3 - 市场

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv3_zb:
    get:
      summary: 实时卡战备V3 - 市场
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【2次】资源

        更新频率：
        - 每分钟更新一次

        lv 参数
        - 0 【11W】机密配置
        - 1 【18W】机密配置
        - 2  【55W】绝密巴克什
        - 3  【60W】绝密航天
        - 4  【24W】适应监狱 （注意：每周五12:00 ~ 周一00:00 开启时间才会计算）
        - 5  【78W】绝密监狱
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: lv
          in: query
          description: '0 :11w | 1: 18W | 2: 55W| 3: 60W | 5: 78W'
          required: false
          example: 0
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      data:
                        type: array
                        items:
                          type: object
                          properties:
                            cz:
                              type: integer
                              description: 差值（假账）
                            data:
                              type: array
                              items:
                                type: object
                                properties:
                                  bl:
                                    type: integer
                                    description: 假账百分比，小于0是便宜
                                  grade:
                                    type: integer
                                    description: 等级
                                  id:
                                    type: integer
                                    description: 唯一ID
                                  jiazhang:
                                    type: integer
                                    description: 是否是假账
                                  jz:
                                    type: integer
                                    description: 假账数值，小于0是便宜，反之为贵
                                  name:
                                    type: string
                                    description: 物品名称
                                  pic:
                                    type: string
                                    description: 图片
                                  price:
                                    type: integer
                                    description: 购买价格
                                  type:
                                    type: string
                                    description: 类型
                                required:
                                  - bl
                                  - grade
                                  - id
                                  - jiazhang
                                  - jz
                                  - name
                                  - pic
                                  - price
                                  - type
                                x-apifox-orders:
                                  - bl
                                  - grade
                                  - id
                                  - jiazhang
                                  - jz
                                  - name
                                  - pic
                                  - price
                                  - type
                            jz:
                              type: integer
                              description: 总战备价值
                            name:
                              type: string
                              description: 配装名称
                            price:
                              type: integer
                              description: 花费价格
                          required:
                            - cz
                            - data
                            - jz
                            - name
                            - price
                          x-apifox-orders:
                            - cz
                            - data
                            - jz
                            - name
                            - price
                      time:
                        type: string
                        description: 配置更新时间
                    required:
                      - data
                      - time
                    x-apifox-orders:
                      - data
                      - time
                required:
                  - code
                  - message
                  - msg
                  - data
                x-apifox-orders:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  data:
                    - cz: -15771
                      data:
                        - bl: -11
                          grade: 1
                          id: 217
                          jiazhang: 1
                          jz: -4075
                          name: Mini-14射手步枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18050000002.png
                          price: 32818
                          type: 枪1
                        - bl: -10
                          grade: 3
                          id: 62
                          jiazhang: 1
                          jz: -4458
                          name: Mini-14增强枪管
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13020000489.png
                          price: 36248
                          type: 枪1-枪管
                        - bl: -26
                          grade: 3
                          id: 516
                          jiazhang: 1
                          jz: -9866
                          name: Mini-14 30发弹匣
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13120000267.png
                          price: 27663
                          type: 枪1-弹匣
                      jz: 115128
                      name: 枪械优先
                      price: 96729
                    - cz: -12106
                      data:
                        - bl: -11
                          grade: 1
                          id: 422
                          jiazhang: 1
                          jz: -4051
                          name: S12K霰弹枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18030000002.png
                          price: 31915
                          type: 枪械1
                        - bl: -11
                          grade: 4
                          id: 486
                          jiazhang: 1
                          jz: -8152
                          name: D6 战术头盔
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11010004001.png
                          price: 61518
                          type: 头盔
                        - bl: -1
                          grade: 1
                          id: 84
                          jiazhang: 1
                          jz: -70
                          name: 运动背包
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11080001002.png
                          price: 3796
                          type: 背包
                        - bl: -2
                          grade: 2
                          id: 425
                          jiazhang: 1
                          jz: -96
                          name: 尼龙挎包
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11070002001.png
                          price: 3165
                          type: 胸挂
                      jz: 112763
                      name: 均衡套装
                      price: 100394
                    - cz: -11283
                      data:
                        - bl: -12
                          grade: 1
                          id: 134
                          jiazhang: 1
                          jz: -11516
                          name: QJB201轻机枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18040000004.png
                          price: 79033
                          type: 枪械1
                        - bl: 0
                          grade: 1
                          id: 524
                          jiazhang: 0
                          jz: 8
                          name: 奔尼帽 (破损)
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11010001003.png
                          price: 1518
                          type: 头盔
                        - bl: -3
                          grade: 2
                          id: 430
                          jiazhang: 1
                          jz: -200
                          name: HT战术背心 (破损)
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11050002002.png
                          price: 5887
                          type: 护甲
                        - bl: -1
                          grade: 1
                          id: 368
                          jiazhang: 1
                          jz: -99
                          name: 旅行背包
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11080001003.png
                          price: 4962
                          type: 背包
                        - bl: 3
                          grade: 3
                          id: 435
                          jiazhang: 0
                          jz: 297
                          name: G01战术弹挂
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11070003004.png
                          price: 9817
                          type: 胸挂
                      jz: 112727
                      name: 胸挂优先
                      price: 101217
                  time: '2025-12-26 16:01:40'
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-396183649-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 实时卡战备V3 - 市场 - ALL

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv3_zb_all:
    get:
      summary: 实时卡战备V3 - 市场 - ALL
      deprecated: false
      description: |-
        本接口在于，需要一次性请求所有战备套装的情况。

        资源扣除逻辑：
        - 请求并数据正常【5次】资源

        更新频率：
        - 每分钟更新一次


        参数
        - 0 【11W】机密配置
        - 1 【18W】机密配置
        - 2  【55W】绝密巴克什
        - 3  【60W】绝密航天
        - 4  【24W】适应监狱 （注意：每周五12:00 ~ 周一00:00 开启时间才会计算）
        - 5  【78W】绝密监狱
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      '0':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: 'null'
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '1':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: 'null'
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '2':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: 'null'
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '3':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: 'null'
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '4':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: string
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '5':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: 'null'
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                    required:
                      - '0'
                      - '1'
                      - '2'
                      - '3'
                      - '4'
                      - '5'
                required:
                  - code
                  - message
                  - msg
                  - data
              examples:
                '1':
                  summary: 成功示例
                  value:
                    code: 0
                    message: 成功
                    msg: 成功
                    data:
                      '0':
                        data:
                          - cz: -12870
                            data:
                              - bl: -13
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 573
                                jiazhang: 1
                                jz: -8191
                                name: M249轻机枪
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/18040000002.png
                                price: 51768
                                type: 枪1
                              - bl: -11
                                exchange: ''
                                exchange_plus: null
                                grade: 4
                                id: 114
                                jiazhang: 1
                                jz: -2618
                                name: 炽火抑制器
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13130000159.png
                                price: 20113
                                type: 枪1-枪口
                              - bl: -9
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 119
                                jiazhang: 1
                                jz: -2854
                                name: 全景红点瞄准镜
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13110000043.png
                                price: 27749
                                type: 枪1-瞄具
                            jz: 113293
                            name: 枪械优先
                            price: 99630
                          - cz: -12305
                            data:
                              - bl: -11
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 442
                                jiazhang: 1
                                jz: -11616
                                name: PKM通用机枪
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/18040000001.png
                                price: 91193
                                type: 枪械1
                              - bl: 1
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 1
                                jiazhang: 0
                                jz: 23
                                name: 老式钢盔 (几乎全新)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11010001001.png
                                price: 1324
                                type: 头盔
                              - bl: -13
                                exchange: ''
                                exchange_plus: null
                                grade: 2
                                id: 239
                                jiazhang: 1
                                jz: -1196
                                name: TG战术防弹衣 (几乎全新)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11050002003.png
                                price: 7678
                                type: 护甲
                            jz: 112984
                            name: 均衡套装
                            price: 100195
                          - cz: -13498
                            data:
                              - bl: -13
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 573
                                jiazhang: 1
                                jz: -8191
                                name: M249轻机枪
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/18040000002.png
                                price: 51768
                                type: 枪械1
                              - bl: -14
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 144
                                jiazhang: 1
                                jz: -6116
                                name: 射手战术背心 (几乎全新)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11050003004.png
                                price: 37417
                                type: 护甲
                              - bl: 2
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 435
                                jiazhang: 0
                                jz: 273
                                name: G01战术弹挂
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11070003004.png
                                price: 9817
                                type: 胸挂
                            jz: 113036
                            name: 胸挂优先
                            price: 99002
                        time: '2026-04-01 11:06:43'
                      '1':
                        data:
                          - cz: -22939
                            data:
                              - bl: -11
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 442
                                jiazhang: 1
                                jz: -11616
                                name: PKM通用机枪
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/18040000001.png
                                price: 91193
                                type: 枪1
                              - bl: -11
                                exchange: ''
                                exchange_plus: null
                                grade: 4
                                id: 114
                                jiazhang: 1
                                jz: -2618
                                name: 炽火抑制器
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13130000159.png
                                price: 20113
                                type: 枪1-枪口
                              - bl: -13
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 281
                                jiazhang: 1
                                jz: -2514
                                name: 稳定AK后握把
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13030000122.png
                                price: 16060
                                type: 枪1-后握把
                              - bl: -14
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 293
                                jiazhang: 1
                                jz: -6532
                                name: PKM壕战短枪管
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13020000414.png
                                price: 37195
                                type: 枪1-枪管
                            jz: 187841
                            name: 枪械优先
                            price: 164561
                          - cz: -22346
                            data:
                              - bl: -11
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 442
                                jiazhang: 1
                                jz: -11616
                                name: PKM通用机枪
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/18040000001.png
                                price: 91193
                                type: 枪械1
                              - bl: -14
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 293
                                jiazhang: 1
                                jz: -6532
                                name: PKM壕战短枪管
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13020000414.png
                                price: 37195
                                type: 枪械1-枪管
                              - bl: -13
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 116
                                jiazhang: 1
                                jz: -1701
                                name: 防暴头盔 (破损)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11010003001.png
                                price: 10944
                                type: 头盔
                              - bl: -4
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 148
                                jiazhang: 1
                                jz: -86
                                name: 安保防弹衣 (几乎全新)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11050001002.png
                                price: 1940
                                type: 护甲
                              - bl: 0
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 368
                                jiazhang: 1
                                jz: -48
                                name: 旅行背包
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11080001003.png
                                price: 4962
                                type: 背包
                              - bl: -12
                                exchange: ''
                                exchange_plus: null
                                grade: 4
                                id: 398
                                jiazhang: 1
                                jz: -2672
                                name: 强袭战术背心
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11070004001.png
                                price: 18920
                                type: 胸挂
                            jz: 187809
                            name: 均衡套装
                            price: 165154
                          - cz: -17668
                            data:
                              - bl: -13
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 573
                                jiazhang: 1
                                jz: -8191
                                name: M249轻机枪
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/18040000002.png
                                price: 51768
                                type: 枪械1
                              - bl: -14
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 322
                                jiazhang: 1
                                jz: -3805
                                name: Cobra准直式瞄准镜
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                                price: 22583
                                type: 枪械1-瞄具
                              - bl: -5
                                exchange: ''
                                exchange_plus: null
                                grade: 4
                                id: 115
                                jiazhang: 1
                                jz: -3142
                                name: 幻影垂直握把
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/13140000033.png
                                price: 51523
                                type: 枪械1-前握把
                              - bl: -10
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 309
                                jiazhang: 1
                                jz: -2974
                                name: DAS 防弹头盔 (几乎全新)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11010003003.png
                                price: 25188
                                type: 头盔
                              - bl: -4
                                exchange: ''
                                exchange_plus: null
                                grade: 1
                                id: 148
                                jiazhang: 1
                                jz: -86
                                name: 安保防弹衣 (几乎全新)
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11050001002.png
                                price: 1940
                                type: 护甲
                              - bl: 0
                                exchange: ''
                                exchange_plus: null
                                grade: 2
                                id: 171
                                jiazhang: 0
                                jz: 46
                                name: 轻型户外背包
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11080002001.png
                                price: 7013
                                type: 背包
                              - bl: 2
                                exchange: ''
                                exchange_plus: null
                                grade: 3
                                id: 435
                                jiazhang: 0
                                jz: 273
                                name: G01战术弹挂
                                pic: >-
                                  https://playerhub.df.qq.com/playerhub/60004/object/11070003004.png
                                price: 9817
                                type: 胸挂
                            jz: 187711
                            name: 胸挂优先
                            price: 169832
                        time: '2026-04-01 11:06:44'
                '2':
                  summary: 成功示例
                  value:
                    code: 0
                    message: 成功
                    msg: 成功
                    data:
                      - grade: 3
                        name: 416轻型枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000112.png
                        price: 10222
                        price_hour: 1703
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: MRGS镂空枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000183.png
                        price: 10152
                        price_hour: 1692
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: okp-7反射瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000061.png
                        price: 7136
                        price_hour: 1189
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 0
                        name: AKM突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000006.png
                        price: 27105
                        price_hour: 1129
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 0
                        name: P90冲锋枪
                        period: 48
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18020000002.png
                        price: 32562
                        price_hour: 678
                        primaryClass: gun
                        secondClass: gunSMG
                        secondClassCN: 冲锋枪
                      - grade: 0
                        name: M249轻机枪
                        period: 20
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18040000002.png
                        price: -10122
                        price_hour: -506
                        primaryClass: gun
                        secondClass: gunLMG
                        secondClassCN: 轻机枪
                      - grade: 3
                        name: 416稳固枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000113.png
                        price: 13529
                        price_hour: 2254
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 2
                        name: 俄式准直二倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000042.png
                        price: 7851
                        price_hour: 1308
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 3
                        name: 微型红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000062.png
                        price: 8400
                        price_hour: 1400
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置战斗红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000079.png
                        price: 8425
                        price_hour: 1404
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: M7实用消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000196.png
                        price: 5644
                        price_hour: 806
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 幻影垂直握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000033.png
                        price: -22608
                        price_hour: -5652
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 3
                        name: QJB201轻机枪
                        period: 20
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18040000004.png
                        price: 4700
                        price_hour: 235
                        primaryClass: gun
                        secondClass: gunLMG
                        secondClassCN: 轻机枪
                      - grade: 4
                        name: 影袭导轨枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000007.png
                        price: -6952
                        price_hour: -1158
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: Osight微型瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000087.png
                        price: 15063
                        price_hour: 2510
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: RK-0前握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000029.png
                        price: 9560
                        price_hour: 2390
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: SR-3M紧凑突击步枪
                        period: 48
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18020000008.png
                        price: 56193
                        price_hour: 1170
                        primaryClass: gun
                        secondClass: gunSMG
                        secondClassCN: 冲锋枪
                      - grade: 4
                        name: UR特种战术枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000110.png
                        price: 4003
                        price_hour: 667
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: 侧置Osight微型瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000088.png
                        price: 14017
                        price_hour: 2336
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 炽火抑制器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000159.png
                        price: 5872
                        price_hour: 838
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 共振人体工程握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000034.png
                        price: 15230
                        price_hour: 3807
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: X25U斜侧战斗握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000035.png
                        price: 10132
                        price_hour: 2533
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 3
                        name: 微型瞄具增高架
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13210000012.png
                        price: 15412
                        price_hour: 2568
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 3
                        name: 全景红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000043.png
                        price: 23295
                        price_hour: 3882
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 3
                        name: XCOG突击3.5倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000053.png
                        price: 4828
                        price_hour: 804
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 光学狙击8倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000056.png
                        price: -1905
                        price_hour: -317
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 先进多口径消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000208.png
                        price: -9224
                        price_hour: -1317
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 战术三角握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000027.png
                        price: 1034
                        price_hour: 258
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: M4A1突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000001.png
                        price: 10351
                        price_hour: 431
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 3
                        name: QCQ171冲锋枪
                        period: 48
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                        price: 30698
                        price_hour: 639
                        primaryClass: gun
                        secondClass: gunSMG
                        secondClassCN: 冲锋枪
                      - grade: 3
                        name: XRO快速反应瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000044.png
                        price: 8316
                        price_hour: 1386
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 6
                        name: 先进白热成像战斗瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000067.png
                        price: -89726
                        price_hour: -14954
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置微型红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000078.png
                        price: -5058
                        price_hour: -843
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 轻语战术消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000164.png
                        price: -5120
                        price_hour: -731
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: K1精英斜角握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000036.png
                        price: 5001
                        price_hour: 1250
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: LA-3C绿色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000006.png
                        price: 8378
                        price_hour: 1861
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: OLIGHT Odin S战术手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000016.png
                        price: -80764
                        price_hour: -17947
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 3
                        name: 闪电导轨枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000006.png
                        price: 4360
                        price_hour: 726
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: 战斗红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000063.png
                        price: 10147
                        price_hour: 1691
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 共振二代前握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000039.png
                        price: 552
                        price_hour: 138
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: DBAL-X2紫色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000010.png
                        price: 8082
                        price_hour: 1796
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 0
                        name: AUG突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000015.png
                        price: 47019
                        price_hour: 1959
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 3
                        name: 侵掠核心枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000108.png
                        price: 8909
                        price_hour: 1484
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: 影袭托芯枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000184.png
                        price: 8245
                        price_hour: 1374
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 3
                        name: ACOG精准六倍镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000089.png
                        price: 20408
                        price_hour: 3401
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 灵眼3/7测距狙击瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000091.png
                        price: 24159
                        price_hour: 4026
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 竞赛阻手器
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000024.png
                        price: 9236
                        price_hour: 2309
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 破晓垂直手电握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000041.png
                        price: -23183
                        price_hour: -5795
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: OLIGHT WARRIOR 3S战术手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000018.png
                        price: 11906
                        price_hour: 2645
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 3
                        name: 多用途战术增高架
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13210000013.png
                        price: 15521
                        price_hour: 2586
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 0
                        name: SCAR-H战斗步枪
                        period: 30
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000021.png
                        price: 40087
                        price_hour: 1336
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 0
                        name: 腾龙突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000038.png
                        price: 29413
                        price_hour: 1225
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 4
                        name: 3/7可调倍率狙击镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000057.png
                        price: 30439
                        price_hour: 5073
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 伸缩脚架握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000038.png
                        price: -25500
                        price_hour: -6375
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 蜂网遮光罩
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13370000001.png
                        price: 433
                        price_hour: 96
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 0
                        name: M14射手步枪
                        period: 30
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000005.png
                        price: 15618
                        price_hour: 520
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
                      - grade: 4
                        name: RK3后握把
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13030000169.png
                        price: 8530
                        price_hour: 1066
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 4
                        name: 视点3倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000073.png
                        price: 19477
                        price_hour: 3246
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置全景红点瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000076.png
                        price: 129
                        price_hour: 21
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 侧置XRO快速反应瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000077.png
                        price: -662
                        price_hour: -110
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 战术垂直握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000032.png
                        price: -4270
                        price_hour: -1067
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: CR棱镜阻手器
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000040.png
                        price: 9542
                        price_hour: 2385
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 3
                        name: 枢机稳固枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000107.png
                        price: 9851
                        price_hour: 1641
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: HAMR组合瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000051.png
                        price: 10973
                        price_hour: 1828
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: LPVO多倍率战斗瞄具
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000069.png
                        price: 22350
                        price_hour: 3725
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 钛金竞赛制退器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000160.png
                        price: 6345
                        price_hour: 906
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 沙暴垂直补偿器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000168.png
                        price: 6033
                        price_hour: 861
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 平衡握把底座
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13330000002.png
                        price: -4507
                        price_hour: -563
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 0
                        name: PKM通用机枪
                        period: 20
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18040000001.png
                        price: 7316
                        price_hour: 365
                        primaryClass: gun
                        secondClass: gunLMG
                        secondClassCN: 轻机枪
                      - grade: 0
                        name: PSG-1射手步枪
                        period: 16
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000031.png
                        price: 32195
                        price_hour: 2012
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
                      - grade: 4
                        name: 堡垒水平补偿器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000167.png
                        price: 10415
                        price_hour: 1487
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 密令斜角握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000030.png
                        price: -11441
                        price_hour: -2860
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: AK-12突击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18010000018.png
                        price: 33805
                        price_hour: 1408
                        primaryClass: gun
                        secondClass: gunRifle
                        secondClassCN: 步枪
                      - grade: 3
                        name: Cobra准直式瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                        price: 7998
                        price_hour: 1333
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 6/12神射手变倍狙击镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000070.png
                        price: -11140
                        price_hour: -1856
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: PEQ-2红色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000005.png
                        price: 7880
                        price_hour: 1751
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: 薪火螺旋消焰器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000209.png
                        price: 11098
                        price_hour: 1585
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 黎明三角手电握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000042.png
                        price: -2601
                        price_hour: -650
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 0
                        name: SVD狙击步枪
                        period: 16
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000004.png
                        price: 31141
                        price_hour: 1946
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
                      - grade: 4
                        name: 侵袭后握把
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13030000112.png
                        price: -1198
                        price_hour: -149
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 4
                        name: 1p-29俄制3倍瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000072.png
                        price: -3885
                        price_hour: -647
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: M157火控光学系统
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000082.png
                        price: 15059
                        price_hour: 2509
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 海神消焰器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000165.png
                        price: 216
                        price_hour: 30
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 死寂消音器
                        period: 7
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000169.png
                        price: 10654
                        price_hour: 1522
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                      - grade: 4
                        name: 相位战斗前握把
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000022.png
                        price: 770
                        price_hour: 192
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 斜角阻手器
                        period: 4
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13140000026.png
                        price: -2411
                        price_hour: -602
                        primaryClass: acc
                        secondClass: accForeGrip
                        secondClassCN: 前握把
                      - grade: 4
                        name: 稳固握把底座
                        period: 8
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13330000001.png
                        price: 18038
                        price_hour: 2254
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                      - grade: 0
                        name: M700狙击步枪
                        period: 24
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18060000009.png
                        price: 31635
                        price_hour: 1318
                        primaryClass: gun
                        secondClass: gunSniper
                        secondClassCN: 狙击步枪
                      - grade: 3
                        name: 骨架狙击枪托
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000185.png
                        price: 12151
                        price_hour: 2025
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                      - grade: 4
                        name: 侦察1.5/5可调瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000052.png
                        price: 28482
                        price_hour: 4747
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 6
                        name: 先进热融合全息瞄准镜
                        period: 6
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000066.png
                        price: -30910
                        price_hour: -5151
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                      - grade: 4
                        name: 耀斑战术手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000007.png
                        price: -95238
                        price_hour: -21164
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: PERST-7蓝色激光镭指
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000009.png
                        price: 8247
                        price_hour: 1832
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 4
                        name: OLIGHT Baldr Pro R多功能手电
                        period: 4.5
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13160000014.png
                        price: -76019
                        price_hour: -16893
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                      - grade: 0
                        name: SKS射手步枪
                        period: 32
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/18050000006.png
                        price: 27520
                        price_hour: 860
                        primaryClass: gun
                        secondClass: gunMP
                        secondClassCN: 精确射手步枪
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-436666629-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 实时卡战备V3 - 军需处兑换+市场

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv3_zb_plus:
    get:
      summary: 实时卡战备V3 - 军需处兑换+市场
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【2次】资源

        更新频率：
        - 每5分钟更新一次，（2分 7分 12分 17分 ....）

        说明：
        - 请以【exchange】代表是兑换物品，如果为 空白 则为市场
        -  【perCount】兑换数量，【purchaseCount】限购次数，【purchaseDuration】刷新时间

        lv 参数
        - 0 【11W】机密配置
        - 1 【18W】机密配置
        - 2  【55W】绝密巴克什
        - 3  【60W】绝密航天
        - 4  【24W】适应监狱 （注意：每周五12:00 ~ 周一00:00 开启时间才会计算）
        - 5  【78W】绝密监狱
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: lv
          in: query
          description: '0 :11w | 1: 18W | 2: 55W| 3: 60W | 5: 78W'
          required: false
          example: 0
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      data:
                        type: array
                        items:
                          type: object
                          properties:
                            cz:
                              type: integer
                              description: 差值（假账）
                            data:
                              type: array
                              items:
                                type: object
                                properties:
                                  bl:
                                    type: integer
                                    description: 假账百分比，小于0是便宜
                                  exchange:
                                    type: string
                                    description: 是否可兑换，空白是市场
                                  exchange_plus:
                                    type: object
                                    properties:
                                      perCount:
                                        type: integer
                                      purchaseCount:
                                        type: string
                                        description: 兑换次数
                                      purchaseDuration:
                                        type: string
                                        description: 兑换所需时间
                                    required:
                                      - perCount
                                      - purchaseCount
                                      - purchaseDuration
                                    x-apifox-orders:
                                      - perCount
                                      - purchaseCount
                                      - purchaseDuration
                                  grade:
                                    type: integer
                                    description: 等级
                                  id:
                                    type: integer
                                    description: 唯一ID
                                  jiazhang:
                                    type: integer
                                    description: 是否是假账
                                  jz:
                                    type: integer
                                    description: 假账数值，小于0是便宜，反之为贵
                                  name:
                                    type: string
                                    description: 物品名称
                                  pic:
                                    type: string
                                    description: 图片
                                  price:
                                    type: integer
                                    description: 购买价格
                                  type:
                                    type: string
                                    description: 类型
                                required:
                                  - bl
                                  - exchange
                                  - exchange_plus
                                  - grade
                                  - id
                                  - jiazhang
                                  - jz
                                  - name
                                  - pic
                                  - price
                                  - type
                                x-apifox-orders:
                                  - bl
                                  - exchange
                                  - exchange_plus
                                  - grade
                                  - id
                                  - jiazhang
                                  - jz
                                  - name
                                  - pic
                                  - price
                                  - type
                            jz:
                              type: integer
                              description: 总战备价值
                            name:
                              type: string
                              description: 配装名称
                            price:
                              type: integer
                              description: 花费价格
                          required:
                            - cz
                            - data
                            - jz
                            - name
                            - price
                          x-apifox-orders:
                            - cz
                            - data
                            - jz
                            - name
                            - price
                      time:
                        type: string
                    required:
                      - data
                      - time
                    x-apifox-orders:
                      - data
                      - time
                required:
                  - code
                  - message
                  - msg
                  - data
                x-apifox-orders:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  data:
                    - cz: -42299
                      data:
                        - bl: -54
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 1次
                            purchaseDuration: 48小时
                          grade: 1
                          id: 50
                          jiazhang: 1
                          jz: -58636
                          name: QCQ171冲锋枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                          price: 49049
                          type: 枪1
                        - bl: -29
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 1次
                            purchaseDuration: 48小时
                          grade: 4
                          id: 378
                          jiazhang: 1
                          jz: -9040
                          name: 新式侍剑重型握把
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13030000181.png
                          price: 21152
                          type: 枪1-后握把
                      jz: 137877
                      name: 枪械优先
                      price: 70201
                    - cz: -42299
                      data:
                        - bl: -54
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 1次
                            purchaseDuration: 48小时
                          grade: 1
                          id: 50
                          jiazhang: 1
                          jz: -58636
                          name: QCQ171冲锋枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                          price: 49049
                          type: 枪1
                        - bl: -29
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 1次
                            purchaseDuration: 48小时
                          grade: 4
                          id: 378
                          jiazhang: 1
                          jz: -9040
                          name: 新式侍剑重型握把
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13030000181.png
                          price: 21152
                          type: 枪1-后握把
                      jz: 137877
                      name: 均衡套装
                      price: 70201
                    - cz: -31397
                      data:
                        - bl: -25
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 12次
                            purchaseDuration: 8小时
                          grade: 1
                          id: 401
                          jiazhang: 1
                          jz: -11590
                          name: MP5冲锋枪
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/18020000001.png
                          price: 33697
                          type: 枪械1
                        - bl: -35
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 12次
                            purchaseDuration: 8小时
                          grade: 3
                          id: 320
                          jiazhang: 1
                          jz: -12091
                          name: MP5斥候长枪管组合
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/13020000430.png
                          price: 21555
                          type: 枪械1-枪管
                        - bl: -15
                          exchange: ''
                          exchange_plus: null
                          grade: 2
                          id: 72
                          jiazhang: 1
                          jz: -966
                          name: 复古摩托头盔 (破损)
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11010002001.png
                          price: 5254
                          type: 头盔
                        - bl: 0
                          exchange: ''
                          exchange_plus: null
                          grade: 1
                          id: 82
                          jiazhang: 0
                          jz: 6
                          name: 安保防弹衣
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11050001002.png
                          price: 2390
                          type: 护甲
                        - bl: -30
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 9次
                            purchaseDuration: 8小时
                          grade: 3
                          id: 596
                          jiazhang: 1
                          jz: -4764
                          name: 3H战术背包
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11080003002.png
                          price: 10888
                          type: 背包
                        - bl: -23
                          exchange: 兑换
                          exchange_plus:
                            perCount: 1
                            purchaseCount: 9次
                            purchaseDuration: 8小时
                          grade: 3
                          id: 435
                          jiazhang: 1
                          jz: -2231
                          name: G01战术弹挂
                          pic: >-
                            https://playerhub.df.qq.com/playerhub/60004/object/11070003004.png
                          price: 7319
                          type: 胸挂
                      jz: 112739
                      name: 胸挂优先
                      price: 81103
                  time: '2026-03-05 11:07:28'
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-423143378-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 实时卡战备V3 - 军需处兑换+市场- ALL

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv3_zb_plus_all:
    get:
      summary: 实时卡战备V3 - 军需处兑换+市场- ALL
      deprecated: false
      description: |-
        本接口在于，需要一次性请求所有战备套装的情况。

        资源扣除逻辑：
        - 请求并数据正常【5次】资源

        更新频率：
        - 每5分钟更新一次，（2分 7分 12分 17分 ....）

        说明：
        - 请以【exchange】代表是兑换物品，如果为 空白 则为市场
        -  【perCount】兑换数量，【purchaseCount】限购次数，【purchaseDuration】刷新时间

        参数
        - 0 【11W】机密配置
        - 1 【18W】机密配置
        - 2  【55W】绝密巴克什
        - 3  【60W】绝密航天
        - 4  【24W】适应监狱 （注意：每周五12:00 ~ 周一00:00 开启时间才会计算）
        - 5  【78W】绝密监狱
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      '0':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: object
                                        properties:
                                          perCount:
                                            type: integer
                                          purchaseCount:
                                            type: string
                                          purchaseDuration:
                                            type: string
                                        required:
                                          - perCount
                                          - purchaseCount
                                          - purchaseDuration
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '1':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: object
                                        properties:
                                          perCount:
                                            type: integer
                                          purchaseCount:
                                            type: string
                                          purchaseDuration:
                                            type: string
                                        required:
                                          - perCount
                                          - purchaseCount
                                          - purchaseDuration
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '2':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: object
                                        properties:
                                          perCount:
                                            type: integer
                                          purchaseCount:
                                            type: string
                                          purchaseDuration:
                                            type: string
                                        required:
                                          - perCount
                                          - purchaseCount
                                          - purchaseDuration
                                        nullable: true
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '3':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: object
                                        properties:
                                          perCount:
                                            type: integer
                                          purchaseCount:
                                            type: string
                                          purchaseDuration:
                                            type: string
                                        required:
                                          - perCount
                                          - purchaseCount
                                          - purchaseDuration
                                        nullable: true
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '4':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: string
                          time:
                            type: string
                        required:
                          - data
                          - time
                      '5':
                        type: object
                        properties:
                          data:
                            type: array
                            items:
                              type: object
                              properties:
                                cz:
                                  type: integer
                                data:
                                  type: array
                                  items:
                                    type: object
                                    properties:
                                      bl:
                                        type: integer
                                      exchange:
                                        type: string
                                      exchange_plus:
                                        type: object
                                        properties:
                                          perCount:
                                            type: integer
                                          purchaseCount:
                                            type: string
                                          purchaseDuration:
                                            type: string
                                        required:
                                          - perCount
                                          - purchaseCount
                                          - purchaseDuration
                                        nullable: true
                                      grade:
                                        type: integer
                                      id:
                                        type: integer
                                      jiazhang:
                                        type: integer
                                      jz:
                                        type: integer
                                      name:
                                        type: string
                                      pic:
                                        type: string
                                      price:
                                        type: integer
                                      type:
                                        type: string
                                    required:
                                      - bl
                                      - exchange
                                      - exchange_plus
                                      - grade
                                      - id
                                      - jiazhang
                                      - jz
                                      - name
                                      - pic
                                      - price
                                      - type
                                jz:
                                  type: integer
                                name:
                                  type: string
                                price:
                                  type: integer
                              required:
                                - cz
                                - data
                                - jz
                                - name
                                - price
                          time:
                            type: string
                        required:
                          - data
                          - time
                    required:
                      - '0'
                      - '1'
                      - '2'
                      - '3'
                      - '4'
                      - '5'
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  '0':
                    data:
                      - cz: -50688
                        data:
                          - bl: -54
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 1次
                              purchaseDuration: 48小时
                            grade: 1
                            id: 50
                            jiazhang: 1
                            jz: -60699
                            name: QCQ171冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                            price: 49849
                            type: 枪1
                          - bl: -34
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 153
                            jiazhang: 1
                            jz: -6187
                            name: 实用消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                            price: 11963
                            type: 枪1-枪口
                        jz: 128698
                        name: 枪械优先
                        price: 61812
                      - cz: -50688
                        data:
                          - bl: -54
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 1次
                              purchaseDuration: 48小时
                            grade: 1
                            id: 50
                            jiazhang: 1
                            jz: -60699
                            name: QCQ171冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                            price: 49849
                            type: 枪1
                          - bl: -34
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 153
                            jiazhang: 1
                            jz: -6187
                            name: 实用消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                            price: 11963
                            type: 枪1-枪口
                        jz: 128698
                        name: 均衡套装
                        price: 61812
                      - cz: -43369
                        data:
                          - bl: -54
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 1次
                              purchaseDuration: 48小时
                            grade: 1
                            id: 50
                            jiazhang: 1
                            jz: -60699
                            name: QCQ171冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                            price: 49849
                            type: 枪1
                          - bl: -34
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 153
                            jiazhang: 1
                            jz: -6187
                            name: 实用消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                            price: 11963
                            type: 枪1-枪口
                          - bl: -23
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 9次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 435
                            jiazhang: 1
                            jz: -2225
                            name: G01战术弹挂
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11070003004.png
                            price: 7319
                            type: 胸挂
                        jz: 138242
                        name: 胸挂优先
                        price: 69131
                    time: '2026-04-01 11:07:28'
                  '1':
                    data:
                      - cz: -75198
                        data:
                          - bl: -54
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 1次
                              purchaseDuration: 48小时
                            grade: 1
                            id: 50
                            jiazhang: 1
                            jz: -60699
                            name: QCQ171冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                            price: 49849
                            type: 枪1
                          - bl: -34
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 153
                            jiazhang: 1
                            jz: -6187
                            name: 实用消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                            price: 11963
                            type: 枪1-枪口
                          - bl: -14
                            exchange: ''
                            exchange_plus: null
                            grade: 3
                            id: 322
                            jiazhang: 1
                            jz: -3805
                            name: Cobra准直式瞄准镜
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                            price: 22583
                            type: 枪1-瞄具
                          - bl: -19
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 378
                            jiazhang: 1
                            jz: -6693
                            name: 新式侍剑重型握把
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13030000181.png
                            price: 27907
                            type: 枪1-后握把
                        jz: 189686
                        name: 枪械优先
                        price: 112302
                      - cz: -48828
                        data:
                          - bl: -8
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 1
                            id: 353
                            jiazhang: 1
                            jz: -3608
                            name: 勇士冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000009.png
                            price: 39970
                            type: 枪械1
                          - bl: -52
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 281
                            jiazhang: 1
                            jz: -9689
                            name: 稳定AK后握把
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13030000122.png
                            price: 8885
                            type: 枪械1-后握把
                          - bl: -28
                            exchange: ''
                            exchange_plus: null
                            grade: 3
                            id: 190
                            jiazhang: 1
                            jz: -7719
                            name: 勇士海狸枪管
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13020000507.png
                            price: 18948
                            type: 枪械1-枪管
                          - bl: -34
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 153
                            jiazhang: 1
                            jz: -6187
                            name: 实用消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                            price: 11963
                            type: 枪械1-枪口
                          - bl: -14
                            exchange: ''
                            exchange_plus: null
                            grade: 3
                            id: 322
                            jiazhang: 1
                            jz: -3805
                            name: Cobra准直式瞄准镜
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                            price: 22583
                            type: 枪械1-瞄具
                          - bl: -37
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 6次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 397
                            jiazhang: 1
                            jz: -12671
                            name: Hvk快拆防弹衣
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11050003002.png
                            price: 21005
                            type: 护甲
                          - bl: -30
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 9次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 596
                            jiazhang: 1
                            jz: -4692
                            name: 3H战术背包
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11080003002.png
                            price: 10888
                            type: 背包
                          - bl: -16
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 9次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 324
                            jiazhang: 1
                            jz: -887
                            name: 简易携行弹挂
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11070003001.png
                            price: 4430
                            type: 胸挂
                        jz: 187930
                        name: 均衡套装
                        price: 138672
                      - cz: -45939
                        data:
                          - bl: -8
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 1
                            id: 353
                            jiazhang: 1
                            jz: -3608
                            name: 勇士冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000009.png
                            price: 39970
                            type: 枪械1
                          - bl: -52
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 281
                            jiazhang: 1
                            jz: -9689
                            name: 稳定AK后握把
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13030000122.png
                            price: 8885
                            type: 枪械1-后握把
                          - bl: -28
                            exchange: ''
                            exchange_plus: null
                            grade: 3
                            id: 190
                            jiazhang: 1
                            jz: -7719
                            name: 勇士海狸枪管
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13020000507.png
                            price: 18948
                            type: 枪械1-枪管
                          - bl: -34
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 12次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 153
                            jiazhang: 1
                            jz: -6187
                            name: 实用消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                            price: 11963
                            type: 枪械1-枪口
                          - bl: -14
                            exchange: ''
                            exchange_plus: null
                            grade: 3
                            id: 322
                            jiazhang: 1
                            jz: -3805
                            name: Cobra准直式瞄准镜
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                            price: 22583
                            type: 枪械1-瞄具
                          - bl: -37
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 6次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 397
                            jiazhang: 1
                            jz: -12671
                            name: Hvk快拆防弹衣
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11050003002.png
                            price: 21005
                            type: 护甲
                          - bl: -30
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 9次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 596
                            jiazhang: 1
                            jz: -4692
                            name: 3H战术背包
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11080003002.png
                            price: 10888
                            type: 背包
                          - bl: -23
                            exchange: 兑换
                            exchange_plus:
                              perCount: 1
                              purchaseCount: 9次
                              purchaseDuration: 8小时
                            grade: 3
                            id: 435
                            jiazhang: 1
                            jz: -2225
                            name: G01战术弹挂
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11070003004.png
                            price: 7319
                            type: 胸挂
                        jz: 192157
                        name: 胸挂优先
                        price: 141561
                    time: '2026-04-01 11:07:29'
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-436671113-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```