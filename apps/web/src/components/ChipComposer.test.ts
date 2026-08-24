import type { ChipDenomination } from "@royal-flush/contracts";
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ChipComposer from "./ChipComposer.vue";

const baseProps = {
  denominations: [5, 10, 20, 50, 100] as ChipDenomination[],
  toCall: 10,
  minimumRaiseBy: 20,
  maximumRaiseBy: 990,
  canCheck: false,
  canRaise: true,
  canAllIn: true,
};

describe("ChipComposer", () => {
  it("combines repeatable chips and submits the original chip array", async () => {
    const wrapper = mount(ChipComposer, { props: baseProps });
    await wrapper.get('[aria-label="增加 20 牌桌分"]').trigger("click");
    await wrapper.get('[aria-label="增加 5 牌桌分"]').trigger("click");

    expect(wrapper.text()).toContain("20 + 5");
    expect(wrapper.text()).toContain("跟注 10");
    expect(wrapper.text()).toContain("额外加注25");
    expect(wrapper.text()).toContain("本轮总投入35");

    const confirm = wrapper.get("button.table-action.raise");
    expect(confirm.attributes("disabled")).toBeUndefined();
    await confirm.trigger("click");
    expect(wrapper.emitted("raise")).toEqual([[[20, 5]]]);
    expect(wrapper.text()).toContain("尚未选择筹码");
  });

  it("keeps confirmation disabled until the minimum raise is reached", async () => {
    const wrapper = mount(ChipComposer, { props: baseProps });
    await wrapper.get('[aria-label="增加 5 牌桌分"]').trigger("click");

    expect(wrapper.text()).toContain("还差 15");
    expect(wrapper.get("button.table-action.raise").attributes("disabled")).toBeDefined();

    await wrapper.get('[aria-label="撤回最后一枚筹码"]').trigger("click");
    expect(wrapper.text()).toContain("尚未选择筹码");
  });

  it("never renders an unconfigured large chip and keeps all-in independent", async () => {
    const wrapper = mount(ChipComposer, { props: { ...baseProps, canRaise: false, maximumRaiseBy: 0 } });

    expect(wrapper.find('[aria-label="增加 200 牌桌分"]').exists()).toBe(false);
    expect(wrapper.get("button.table-action.raise").attributes("disabled")).toBeDefined();
    await wrapper.get("button.table-action.all-in").trigger("click");
    expect(wrapper.emitted("allIn")).toHaveLength(1);
  });
});
