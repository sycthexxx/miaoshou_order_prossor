package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
)

const supplierChatPath = "/app/ocms-fusion-components-1688/def_cbu_web_im/index.html"

func saveSupplierChat(tx *sql.Tx, account string, supplier int64, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Host != "air.1688.com" || u.User != nil || u.EscapedPath() != supplierChatPath {
		return fmt.Errorf("1688 客服链接格式无效")
	}
	params := u.Query()
	nick, ok := strings.CutPrefix(params.Get("touid"), "cnalichn")
	if !ok || strings.TrimSpace(nick) == "" || params.Get("siteid") != "cnalichn" {
		return fmt.Errorf("1688 客服链接缺少卖家账号")
	}
	u.RawQuery = url.Values{"touid": {"cnalichn" + nick}, "siteid": {"cnalichn"}, "status": {"1"}}.Encode()
	u.Fragment = ""
	_, err = tx.Exec(`INSERT INTO supplier_chat_links(account_id,supplier_id,chat_url,seller_nick) VALUES(?,?,?,?)
		ON CONFLICT(account_id,supplier_id) DO UPDATE SET chat_url=excluded.chat_url,seller_nick=excluded.seller_nick`,
		account, supplier, u.String(), nick)
	return err
}

func shippedSupplierContacts(db rowsQuerier, account string, factories []*shippedFactory) error {
	rows, err := db.Query(`SELECT s.name,c.seller_nick,c.chat_url FROM supplier_chat_links c
		JOIN suppliers s ON s.id=c.supplier_id AND s.account_id=c.account_id WHERE c.account_id=? ORDER BY s.id`, account)
	if err != nil {
		return err
	}
	defer rows.Close()
	byName, byNick := map[string]string{}, map[string]string{}
	for rows.Next() {
		var name, nick, link string
		if err := rows.Scan(&name, &nick, &link); err != nil {
			return err
		}
		byName[name], byNick[nick] = link, link
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, factory := range factories {
		if factory.Name == "未指定厂家" || (!factory.has1688 && factory.OfferID == "") {
			continue
		}
		factory.ChatURL = firstText(byName[factory.Name], byNick[factory.Name])
	}
	return nil
}
