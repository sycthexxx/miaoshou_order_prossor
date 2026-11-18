import {ref, onMounted, onBeforeUnmount, watch} from 'vue/dist/vue.esm-bundler.js';
import flatpickr from 'flatpickr';
import {Mandarin} from 'flatpickr/dist/l10n/zh.js';
import 'flatpickr/dist/flatpickr.css';
import './procurement-calendar.css';

export default {
 props: ['modelValue', 'label', 'min', 'max'],
 emits: ['update:modelValue'],
 setup(props, {emit}) {
  const input = ref(null);
  let calendar;
  const today = () => flatpickr.formatDate(new Date(), 'Y-m-d');
  let todayButton;
  function updateTodayButton() {
   const day = today();
   todayButton.disabled = Boolean((props.min && day < props.min) || (props.max && day > props.max));
  }
  onMounted(() => {
   calendar = flatpickr(input.value, {
    locale: Mandarin, dateFormat: 'Y-m-d', disableMobile: true,
    defaultDate: props.modelValue || undefined,
    minDate: props.min || undefined, maxDate: props.max || undefined,
    onChange: (_, value) => emit('update:modelValue', value),
   });
   calendar.calendarContainer.classList.add('procurement-calendar');
   const footer = document.createElement('div');
   footer.className = 'procurement-calendar-footer';
   function action(label, select) {
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = label;
    button.onclick = () => {
     select();
     calendar.close();
    };
    footer.append(button);
    return button;
   }
   action('清除', () => calendar.clear());
   todayButton = action('今天', () => calendar.setDate(today(), true));
   updateTodayButton();
   calendar.calendarContainer.append(footer);
  });
  watch(() => props.modelValue, value => {
   if (calendar && input.value.value !== value) calendar.setDate(value || null);
  });
  watch(() => [props.min, props.max], ([min, max]) => {
   calendar.set({minDate: min || undefined, maxDate: max || undefined});
   updateTodayButton();
  });
  onBeforeUnmount(() => calendar?.destroy());
  return {input, open: () => calendar.open()};
 },
 template: `<label class="procurement-date-field" @click="open">
  <span>{{label}}</span>
  <input ref="input" type="text" readonly placeholder="选择日期" aria-haspopup="dialog" :aria-label="label" @keydown.space.prevent="open">
  <i aria-hidden="true"></i>
 </label>`,
};
