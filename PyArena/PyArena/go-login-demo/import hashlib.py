import hashlib
import hmac

def tinh_ket_qua_van_choi(server_seed: str, client_seed: str, nonce: int) -> float:
    """
    Mô phỏng thuật toán Provably Fair tạo ra hệ số nhân (Multiplier)
    """
    # 1. Kết hợp các hạt giống bằng thuật toán băm HMAC-SHA512
    message = f"{client_seed}:{nonce}".encode('utf-8')
    key = server_seed.encode('utf-8')
    hash_hex = hmac.new(key, message, hashlib.sha512).hexdigest()
    
    # 2. Lấy 13 ký tự đầu tiên của chuỗi Hex (tương đương 52-bit ngẫu nhiên)
    sub_hash = hash_hex[:13]
    so_ngau_nhien = int(sub_hash, 16) # Chuyển từ hệ 16 sang hệ 10
    
    # Giá trị tối đa của 52-bit là 2^52
    max_val = 2 ** 52
    
    # 3. Cài đặt lợi thế nhà cái (Ví dụ: 3% số ván sẽ nổ ngay lập tức ở 1.00x)
    if so_ngau_nhien % 33 == 0:
        return 1.00
        
    # 4. Công thức toán học chuyển đổi số ngẫu nhiên thành hệ số nhân
    # (Đảm bảo phân phối xác suất: hệ số càng cao thì xác suất xuất hiện càng hiếm)
    he_so = (100 * max_val - so_ngau_nhien) / (max_val - so_ngau_nhien)
    
    # Làm tròn xuống 2 chữ số thập phân (kèm theo cắt phế lợi thế nhà cái)
    ket_qua = int(he_so) / 100.0
    return max(1.00, ket_qua)

# --- CHẠY THỬ MÔ PHỎNG 10 VÁN ---
server_seed_bi_mat = "a9f8e7d6c5b4a3f2e1d0987654321abc" # Nằm trên Server, người chơi không biết trước
client_seed_cong_khai = "user123_user456_user789"       # Ghép từ những người chơi trong phòng

print("Mô phỏng kết quả 10 ván liên tiếp:")
for ván in range(1, 11):
    ket_qua = tinh_ket_qua_van_choi(server_seed_bi_mat, client_seed_cong_khai, ván)
    print(f"Ván {ván:2d} | Kết quả nổ ở: {ket_qua:.2f}x")