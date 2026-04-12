const WHATSAPP_WINDOW_NAME = 'admin-whatsapp-chat'

export function buildWhatsAppLink(phone: string) {
  const digits = phone.replace(/\D/g, '')
  let normalized = digits
  if (normalized.startsWith('00')) {
    normalized = normalized.slice(2)
  }
  if (normalized.startsWith('0')) {
    normalized = `20${normalized.slice(1)}`
  }
  return `https://api.whatsapp.com/send?phone=${normalized}`
}

export function openWhatsAppLink(url: string) {
  if (typeof window === 'undefined') {
    return false
  }

  const popup = window.open(url, WHATSAPP_WINDOW_NAME)
  if (!popup) {
    return false
  }

  popup.focus()
  return true
}
