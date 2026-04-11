const WHATSAPP_WINDOW_NAME = 'admin-whatsapp-chat'

let whatsappWindow: Window | null = null

export function buildWhatsAppLink(phone: string) {
  const digits = phone.replace(/\D/g, '')
  let normalized = digits
  if (normalized.startsWith('00')) {
    normalized = normalized.slice(2)
  }
  if (normalized.startsWith('0')) {
    normalized = `20${normalized.slice(1)}`
  }
  return `https://wa.me/${normalized}`
}

export function openWhatsAppLink(url: string) {
  if (typeof window === 'undefined') {
    return false
  }

  const popup = whatsappWindow && !whatsappWindow.closed
    ? whatsappWindow
    : window.open('', WHATSAPP_WINDOW_NAME)

  if (!popup) {
    return false
  }

  whatsappWindow = popup

  try {
    whatsappWindow.opener = null
  } catch {
    // Ignore cross-origin or browser-specific opener restrictions.
  }

  whatsappWindow.location.href = url
  whatsappWindow.focus()
  return true
}
